package repository

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"robot-cell-safety-envelope-validator/backend/internal/model"
)

func openR002DB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.ValidationRun{}, &model.MotionProgram{}, &model.RobotCell{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC()
	runs := []model.ValidationRun{
		{MotionProgramID: 1, ZoneSnapshot: "{}", ProgramSnapshot: "{}", AlgorithmVersion: "v1",
			InputHash: "h1", IdempotencyKey: "key-a-11111111", Attempt: 1,
			CollisionEventsJSON: "[]", InterlockFindingsJSON: "[]", RiskScore: 0,
			ValidationStatus: "passed", Explanation: "x", RequestedBy: 1, StartedAt: now},
		{MotionProgramID: 1, ZoneSnapshot: "{}", ProgramSnapshot: "{}", AlgorithmVersion: "v1",
			InputHash: "h2", IdempotencyKey: "key-b-22222222", Attempt: 1,
			CollisionEventsJSON: "[]", InterlockFindingsJSON: "[]", RiskScore: 0,
			ValidationStatus: "failed", Explanation: "y", RequestedBy: 1, StartedAt: now.Add(-time.Minute)},
	}
	if err := db.Create(&runs).Error; err != nil {
		t.Fatalf("seed runs: %v", err)
	}
	return db
}

func TestValidationRunListHonorsPageSize(t *testing.T) {
	db := openR002DB(t)
	repo := NewValidationRunRepository(db)
	zones, total, err := repo.List(1, 1, 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(zones) != 1 {
		t.Fatalf("len = %d, want 1 (page_size must be honored)", len(zones))
	}
}

func TestValidationRunListNoCrossCallPollution(t *testing.T) {
	db := openR002DB(t)
	repo := NewValidationRunRepository(db)
	first, _, err := repo.List(1, 50, 0, "")
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	firstLen := len(first)
	if firstLen == 0 {
		t.Fatal("first list empty; setup invalid")
	}
	// 修改第一条记录的 status
	if err := db.Model(&model.ValidationRun{}).Where("idempotency_key = ?", first[0].IdempotencyKey).Update("validation_status", "reviewed").Error; err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, _, err := repo.List(1, 50, 0, ""); err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(first) != firstLen {
		t.Fatalf("first list result changed length after another call: %d -> %d", firstLen, len(first))
	}
	for _, run := range first {
		if run.ValidationStatus != "passed" && run.ValidationStatus != "failed" {
			t.Fatalf("first list result polluted by later call: %+v", first)
		}
	}
}
