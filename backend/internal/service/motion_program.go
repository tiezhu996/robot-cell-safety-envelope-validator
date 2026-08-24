package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"robot-cell-safety-envelope-validator/backend/internal/algorithm"
	"robot-cell-safety-envelope-validator/backend/internal/constants"
	"robot-cell-safety-envelope-validator/backend/internal/dto"
	"robot-cell-safety-envelope-validator/backend/internal/geometry"
	"robot-cell-safety-envelope-validator/backend/internal/model"
	"robot-cell-safety-envelope-validator/backend/internal/repository"
)

type MotionProgramService struct {
	db         *gorm.DB
	repository *repository.MotionProgramRepository
	cells      *repository.RobotCellRepository
	system     *SystemService
}

func NewMotionProgramService(db *gorm.DB, repository *repository.MotionProgramRepository, cells *repository.RobotCellRepository, system *SystemService) *MotionProgramService {
	return &MotionProgramService{db: db, repository: repository, cells: cells, system: system}
}

func (service *MotionProgramService) Create(request dto.CreateMotionProgramRequest, actor dto.Actor, requestID string) (dto.MotionProgramResponse, error) {
	cell, err := service.cells.Get(request.RobotCellID)
	if err != nil {
		return dto.MotionProgramResponse{}, MapRepositoryError("robot cell", err)
	}
	if cell.CellState == constants.CellStateInactive {
		return dto.MotionProgramResponse{}, Conflict("cell_inactive", "cannot import a program into an inactive cell", repository.ErrStateConflict)
	}
	if err := geometry.ValidateTrajectory(request.Trajectory); err != nil {
		return dto.MotionProgramResponse{}, Unprocessable("invalid_trajectory", err.Error(), err)
	}
	if validationErrors := algorithm.ValidateInterlockEvents(request.InterlockSequence); len(validationErrors) > 0 {
		return dto.MotionProgramResponse{}, Unprocessable("invalid_interlock_sequence", strings.Join(validationErrors, "; "), nil)
	}
	trajectoryJSON, _ := json.Marshal(request.Trajectory)
	interlockJSON, _ := json.Marshal(request.InterlockSequence)
	program := model.MotionProgram{
		RobotCellID: request.RobotCellID, ProgramCode: strings.ToUpper(strings.TrimSpace(request.ProgramCode)), Version: request.Version,
		TrajectoryJSON: string(trajectoryJSON), ToolRadiusMM: request.ToolRadiusMM, PayloadRadiusMM: request.PayloadRadiusMM,
		InterlockSequenceJSON: string(interlockJSON), SourceChecksum: checksumProgram(request),
		ProgramState: constants.ProgramStateUploaded, UploadedBy: actor.ID, UploadedAt: time.Now().UTC(),
	}
	if err := service.repository.Create(&program); err != nil {
		if repository.IsUniqueViolation(err) {
			return dto.MotionProgramResponse{}, Conflict("duplicate_program_version", "program_code and version already exist", err)
		}
		return dto.MotionProgramResponse{}, Internal("could not import motion program", err)
	}
	if err := service.system.RecordAudit(actor, requestID, "motion_program.uploaded", "motion_program", auditID(program.ID), map[string]any{
		"program_code": program.ProgramCode, "version": program.Version, "point_count": len(request.Trajectory), "source_checksum": program.SourceChecksum,
	}, nil, programSummary(program)); err != nil {
		return dto.MotionProgramResponse{}, err
	}
	return service.Get(program.ID)
}

func (service *MotionProgramService) Get(id uint) (dto.MotionProgramResponse, error) {
	program, err := service.repository.Get(id)
	if err != nil {
		return dto.MotionProgramResponse{}, MapRepositoryError("motion program", err)
	}
	return programResponse(program)
}

func (service *MotionProgramService) List(page, pageSize int, cellID uint, state string) ([]dto.MotionProgramResponse, dto.PageMeta, error) {
	programs, total, err := service.repository.List(page, pageSize, cellID, state)
	if err != nil {
		return nil, dto.PageMeta{}, Internal("could not list motion programs", err)
	}
	responses := make([]dto.MotionProgramResponse, 0, len(programs))
	for _, program := range programs {
		response, err := programResponse(program)
		if err != nil {
			return nil, dto.PageMeta{}, Internal("stored program payload is invalid", err)
		}
		responses = append(responses, response)
	}
	return responses, PageMeta(page, pageSize, total), nil
}

func (service *MotionProgramService) Transition(id uint, target string, actor dto.Actor, requestID string) (dto.MotionProgramResponse, error) {
	if !constants.ValidProgramState(target) {
		return dto.MotionProgramResponse{}, BadRequest("invalid_target_state", fmt.Sprintf("target state %q is not a known program state", target))
	}
	before, err := service.repository.Get(id)
	if err != nil {
		return dto.MotionProgramResponse{}, MapRepositoryError("motion program", err)
	}
	if !constants.CanTransitionProgram(before.ProgramState, target) {
		return dto.MotionProgramResponse{}, Conflict("invalid_program_transition", fmt.Sprintf("program cannot transition from %s to %s", before.ProgramState, target), repository.ErrStateConflict)
	}
	// Parsing is a side-effecting transition: run semantic checks first and
	// route failures into rejected rather than parsed.
	if target == constants.ProgramStateParsed {
		if parseErrors := service.parseErrors(before); len(parseErrors) > 0 {
			return service.rejectParse(id, before, parseErrors, actor, requestID)
		}
	}
	err = service.db.Transaction(func(tx *gorm.DB) error {
		repo := service.repository.WithDB(tx)
		// Activating supersedes the prior active program in the SAME cell, so
		// a cell never has two active programs at once. Other cells are
		// untouched.
		if target == constants.ProgramStateActive {
			if err := repo.SupersedeActive(before.RobotCellID, before.ID); err != nil {
				return err
			}
		}
		if err := repo.Transition(id, before.ProgramState, target); err != nil {
			return err
		}
		after, err := repo.Get(id)
		if err != nil {
			return err
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "motion_program.state_changed", "motion_program", auditID(id), map[string]any{"target_state": target}, programSummary(before), programSummary(after))
	})
	if err != nil {
		if errors.Is(err, repository.ErrStateConflict) {
			return dto.MotionProgramResponse{}, Conflict("state_conflict", "program state changed concurrently", err)
		}
		return dto.MotionProgramResponse{}, Internal("could not transition motion program", err)
	}
	return service.Get(id)
}

// rejectParse moves a program from its current state to rejected after parse
// validation fails. It runs in a transaction and records a parse_rejected
// audit event describing the failures.
func (service *MotionProgramService) rejectParse(id uint, before model.MotionProgram, parseErrors []string, actor dto.Actor, requestID string) (dto.MotionProgramResponse, error) {
	var after model.MotionProgram
	err := service.db.Transaction(func(tx *gorm.DB) error {
		repo := service.repository.WithDB(tx)
		if err := repo.Transition(id, before.ProgramState, constants.ProgramStateRejected); err != nil {
			return err
		}
		loaded, err := repo.Get(id)
		if err != nil {
			return err
		}
		after = loaded
		return service.system.RecordAuditTx(tx, actor, requestID, "motion_program.parse_rejected", "motion_program", auditID(id), map[string]any{"errors": parseErrors}, programSummary(before), programSummary(after))
	})
	if err != nil {
		if errors.Is(err, repository.ErrStateConflict) {
			return dto.MotionProgramResponse{}, Conflict("state_conflict", "program state changed while rejecting parse", err)
		}
		return dto.MotionProgramResponse{}, Internal("could not reject motion program", err)
	}
	return dto.MotionProgramResponse{}, Unprocessable("program_parse_failed", strings.Join(parseErrors, "; "), nil)
}

func (service *MotionProgramService) parseErrors(program model.MotionProgram) []string {
	var trajectory []dto.TrajectoryPoint
	if err := json.Unmarshal([]byte(program.TrajectoryJSON), &trajectory); err != nil {
		return []string{"trajectory JSON cannot be decoded"}
	}
	errorsFound := make([]string, 0)
	if err := geometry.ValidateTrajectory(trajectory); err != nil {
		errorsFound = append(errorsFound, err.Error())
	}
	var events []dto.InterlockEvent
	if err := json.Unmarshal([]byte(program.InterlockSequenceJSON), &events); err != nil {
		errorsFound = append(errorsFound, "interlock JSON cannot be decoded")
	} else {
		errorsFound = append(errorsFound, algorithm.ValidateInterlockEvents(events)...)
	}
	return errorsFound
}

func checksumProgram(request dto.CreateMotionProgramRequest) string {
	canonical := struct {
		CellID     uint                  `json:"cell_id"`
		Code       string                `json:"code"`
		Version    int                   `json:"version"`
		Trajectory []dto.TrajectoryPoint `json:"trajectory"`
		ToolRadius float64               `json:"tool_radius"`
		Payload    float64               `json:"payload_radius"`
		Interlocks []dto.InterlockEvent  `json:"interlocks"`
	}{request.RobotCellID, strings.ToUpper(strings.TrimSpace(request.ProgramCode)), request.Version, request.Trajectory, request.ToolRadiusMM, request.PayloadRadiusMM, request.InterlockSequence}
	encoded, _ := json.Marshal(canonical)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func programResponse(program model.MotionProgram) (dto.MotionProgramResponse, error) {
	var trajectory []dto.TrajectoryPoint
	if err := json.Unmarshal([]byte(program.TrajectoryJSON), &trajectory); err != nil {
		return dto.MotionProgramResponse{}, err
	}
	var interlocks []dto.InterlockEvent
	if err := json.Unmarshal([]byte(program.InterlockSequenceJSON), &interlocks); err != nil {
		return dto.MotionProgramResponse{}, err
	}
	return dto.MotionProgramResponse{
		ID: program.ID, RobotCellID: program.RobotCellID, RobotCellCode: program.RobotCell.CellCode,
		ProgramCode: program.ProgramCode, Version: program.Version, Trajectory: trajectory,
		ToolRadiusMM: program.ToolRadiusMM, PayloadRadiusMM: program.PayloadRadiusMM,
		InterlockSequence: interlocks, SourceChecksum: program.SourceChecksum,
		ProgramState: program.ProgramState, UploadedBy: program.UploadedBy, UploadedAt: program.UploadedAt,
	}, nil
}

func programSummary(program model.MotionProgram) map[string]any {
	return map[string]any{"program_code": program.ProgramCode, "version": program.Version, "program_state": program.ProgramState, "source_checksum": program.SourceChecksum, "robot_cell_id": program.RobotCellID}
}
