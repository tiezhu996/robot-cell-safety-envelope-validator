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

func openR003DB(t *testing.T) *gorm.DB {
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
	cell := model.RobotCell{
		CellCode: "CELL-X", Name: "cell x", LayoutGeoJSON: `{"type":"Polygon"}`,
		RobotModel: "R", ControllerModel: "C", MaxReachMM: 1000, OwnerTeam: "T",
		CellState: constants.CellStateFrozen, LayoutVersion: 1, CreatedBy: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&cell).Error; err != nil {
		t.Fatalf("seed cell: %v", err)
	}
	zones := []model.SafetyZone{
		{RobotCellID: cell.ID, Name: "alpha", ZoneType: constants.ZoneTypeOperating,
			PolygonGeoJSON: `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`,
			MinHeightMM: 0, MaxHeightMM: 1000, SpeedLimitMMS: 100, AccessRule: "x",
			ZoneState: constants.ZoneStateActive, Version: 1, CreatedBy: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{RobotCellID: cell.ID, Name: "beta", ZoneType: constants.ZoneTypeRestricted,
			PolygonGeoJSON: `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`,
			MinHeightMM: 0, MaxHeightMM: 1000, SpeedLimitMMS: 50, AccessRule: "x",
			ZoneState: constants.ZoneStateActive, Version: 1, CreatedBy: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	if err := db.Create(&zones).Error; err != nil {
		t.Fatalf("seed zones: %v", err)
	}
	return db
}

func TestSafetyZoneRepositoryListHonorsPageSize(t *testing.T) {
	db := openR003DB(t)
	repo := NewSafetyZoneRepository(db)
	zones, total, err := repo.List(1, 1, 0, "", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(zones) != 1 {
		t.Fatalf("len(zones) = %d, want 1 (page_size must be honored)", len(zones))
	}
}
