package repository

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"robot-cell-safety-envelope-validator/backend/internal/constants"
	"robot-cell-safety-envelope-validator/backend/internal/model"
)

func openR006DB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.RobotCell{}, &model.SafetyZone{}, &model.MotionProgram{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRobotCellTransitionEnforcesState(t *testing.T) {
	db := openR006DB(t)
	cell := model.RobotCell{
		CellCode: "CELL-Y", Name: "cell y", LayoutGeoJSON: `{"type":"Polygon"}`,
		RobotModel: "R", ControllerModel: "C", MaxReachMM: 1000, OwnerTeam: "T",
		CellState: constants.CellStateFrozen, LayoutVersion: 1, CreatedBy: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&cell).Error; err != nil {
		t.Fatalf("seed cell: %v", err)
	}
	repo := NewRobotCellRepository(db)
	// 从错误的当前状态（draft）迁移已冻结单元，必须被拒绝
	if err := repo.Transition(cell.ID, constants.CellStateDraft, constants.CellStateFrozen); err == nil {
		t.Fatal("transition from a mismatched current state must fail")
	}
	// 合法迁移 frozen -> inactive 必须成功
	if err := repo.Transition(cell.ID, constants.CellStateFrozen, constants.CellStateInactive); err != nil {
		t.Fatalf("legal transition frozen -> inactive failed: %v", err)
	}
}
