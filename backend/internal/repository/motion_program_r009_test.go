package repository

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"robot-cell-safety-envelope-validator/backend/internal/constants"
	"robot-cell-safety-envelope-validator/backend/internal/model"
)

func openR009DB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.MotionProgram{}, &model.RobotCell{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cell := model.RobotCell{
		CellCode: "CELL-Z", Name: "cell z", LayoutGeoJSON: `{"type":"Polygon"}`,
		RobotModel: "R", ControllerModel: "C", MaxReachMM: 1000, OwnerTeam: "T",
		CellState: constants.CellStateFrozen, LayoutVersion: 1, CreatedBy: 1,
	}
	if err := db.Create(&cell).Error; err != nil {
		t.Fatalf("seed cell: %v", err)
	}
	return db
}

func TestMotionProgramGetPreservesRecordNotFound(t *testing.T) {
	db := openR009DB(t)
	repo := NewMotionProgramRepository(db)
	_, err := repo.Get(999999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("errors.Is(err, gorm.ErrRecordNotFound) = false, got %v", err)
	}
}
