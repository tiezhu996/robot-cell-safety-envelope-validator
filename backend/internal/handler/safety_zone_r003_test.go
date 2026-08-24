package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"robot-cell-safety-envelope-validator/backend/internal/config"
	"robot-cell-safety-envelope-validator/backend/internal/constants"
	"robot-cell-safety-envelope-validator/backend/internal/dto"
	"robot-cell-safety-envelope-validator/backend/internal/model"
	"robot-cell-safety-envelope-validator/backend/internal/repository"
	"robot-cell-safety-envelope-validator/backend/internal/router"
	"robot-cell-safety-envelope-validator/backend/internal/service"
)

func r003Setup(t *testing.T) (*gin.Engine, string, *gorm.DB) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r003-test-secret-0123456789abcdef",
		JWTTTL:             2 * time.Hour,
		CORSOrigin:         "http://localhost:18533",
		LogLevel:           "info",
		RateLimitPerMinute: 10000,
		AlgorithmVersion:   "envelope-2d-height-v1.0",
	}
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	engine := router.New(db, cfg)
	body := bytes.NewBufferString(`{"username":"engineer","password":"Safety#533"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d", w.Code)
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return engine, resp.Data.Token, db
}

func r003CreateZone(t *testing.T, engine *gin.Engine, token, name string, cellID uint) uint {
	t.Helper()
	payload := fmt.Sprintf(`{"robot_cell_id":%d,"name":"%s","zone_type":"operating","polygon_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"min_height_mm":0,"max_height_mm":1000,"speed_limit_mm_s":100,"access_rule":"none"}`, cellID, name)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create zone %s status %d", name, w.Code)
	}
	var resp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode zone: %v", err)
	}
	return resp.Data.ID
}

func TestListSafetyZonesFilterIsolation(t *testing.T) {
	engine, token, db := r003Setup(t)
	var cell model.RobotCell
	if err := db.Where("cell_code = ?", "CELL-A17").First(&cell).Error; err != nil {
		t.Fatalf("seeded cell not found: %v", err)
	}
	a := r003CreateZone(t, engine, token, "filter-zone-a", cell.ID)
	b := r003CreateZone(t, engine, token, "filter-zone-b", cell.ID)
	// activate both then deactivate b
	for _, id := range []uint{a, b} {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/zones/%d/activate", id), bytes.NewBufferString(`{"version":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("activate %d status %d", id, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/zones/%d/deactivate", b), bytes.NewBufferString(`{"version":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("deactivate status %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones?state=inactive", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d", w.Code)
	}
	var listResp struct {
		Data []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, item := range listResp.Data {
		if item.ID == a {
			t.Fatalf("active zone %q leaked into inactive filter result", item.Name)
		}
	}
	found := false
	for _, item := range listResp.Data {
		if item.ID == b {
			found = true
		}
	}
	if !found {
		t.Fatalf("inactive zone filter-zone-b missing: %+v", listResp.Data)
	}
}

func TestSafetyZoneServiceListNoCrossRequestPollution(t *testing.T) {
	_, _, db := r003Setup(t)
	cfg := config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "_svc?mode=memory&cache=shared",
		DBAutoMigrate: true,
		JWTSecret: "r003-test-secret-0123456789abcdef",
		JWTTTL:    2 * time.Hour,
		RateLimitPerMinute: 10000,
		AlgorithmVersion: "envelope-2d-height-v1.0",
	}
	db2, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	systemRepo := repository.NewSystemRepository(db2)
	systemSvc := service.NewSystemService(systemRepo, cfg.JWTSecret, cfg.JWTTTL)
	cellRepo := repository.NewRobotCellRepository(db2)
	zoneRepo := repository.NewSafetyZoneRepository(db2)
	zoneSvc := service.NewSafetyZoneService(zoneRepo, cellRepo, systemSvc)
	var cell model.RobotCell
	if err := db2.Where("cell_code = ?", "CELL-A17").First(&cell).Error; err != nil {
		t.Fatalf("seeded cell not found: %v", err)
	}
	actor := dto.Actor{ID: 2, Username: "engineer", Role: constants.RoleSafetyEngineer}
	created, err := zoneSvc.Create(dto.CreateSafetyZoneRequest{
		RobotCellID: cell.ID, Name: "alpha", ZoneType: constants.ZoneTypeOperating,
		PolygonGeoJSON: json.RawMessage(`{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]}`),
		MinHeightMM: 0, MaxHeightMM: 1000, SpeedLimitMMS: 100, AccessRule: "none",
	}, actor, "r003-req-1")
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	first, _, err := zoneSvc.List(1, 50, cell.ID, "", "")
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	firstAlpha := -1
	for i := range first {
		if first[i].Name == "alpha" {
			firstAlpha = i
			break
		}
	}
	if firstAlpha < 0 {
		t.Fatalf("first list missing alpha: %+v", first)
	}
	if err := db2.Model(&model.SafetyZone{}).Where("id = ?", created.ID).Update("name", "alpha-v2").Error; err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, _, err := zoneSvc.List(1, 50, cell.ID, "", ""); err != nil {
		t.Fatalf("second list: %v", err)
	}
	if first[firstAlpha].Name != "alpha" {
		t.Fatalf("first list result was polluted by a later request: got %q", first[firstAlpha].Name)
	}
	_ = db
}
