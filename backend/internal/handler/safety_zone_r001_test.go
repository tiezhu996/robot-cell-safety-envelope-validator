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

	"robot-cell-safety-envelope-validator/backend/internal/config"
	"robot-cell-safety-envelope-validator/backend/internal/router"
)

func r001Router(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r001-test-secret-0123456789abcdef",
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
	login := func(username, password string) string {
		body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login %s status %d", username, w.Code)
		}
		var resp struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode login: %v", err)
		}
		return resp.Data.Token
	}
	return engine, login("engineer", "Safety#533"), login("auditor", "Audit#533")
}

func r001CreateZone(t *testing.T, engine *gin.Engine, token, zoneType, name string) (int, uint) {
	t.Helper()
	payload := fmt.Sprintf(`{"robot_cell_id":1,"name":"%s","zone_type":"%s","polygon_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"min_height_mm":0,"max_height_mm":1000,"speed_limit_mm_s":100,"access_rule":"none"}`, name, zoneType)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		return w.Code, 0
	}
	var resp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode zone: %v", err)
	}
	return w.Code, resp.Data.ID
}

func TestCreateEscapeZoneAccepted(t *testing.T) {
	engine, engToken, _ := r001Router(t)
	code, _ := r001CreateZone(t, engine, engToken, "escape", "escape-route-1")
	if code != http.StatusCreated {
		t.Fatalf("create escape zone status = %d, want 201", code)
	}
}

func TestUpdateZoneToEscapeAccepted(t *testing.T) {
	engine, engToken, _ := r001Router(t)
	code, id := r001CreateZone(t, engine, engToken, "operating", "upd-to-escape-1")
	if code != http.StatusCreated {
		t.Fatalf("create operating zone status %d", code)
	}
	payload := fmt.Sprintf(`{"name":"upd-to-escape-1","zone_type":"escape","polygon_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"min_height_mm":0,"max_height_mm":1000,"speed_limit_mm_s":100,"access_rule":"none","version":1}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/zones/%d", id), bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+engToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update zone to escape status = %d, want 200", w.Code)
	}
}

func TestUpdateZoneWithVersionOneAccepted(t *testing.T) {
	engine, engToken, _ := r001Router(t)
	code, id := r001CreateZone(t, engine, engToken, "operating", "version-one-1")
	if code != http.StatusCreated {
		t.Fatalf("create zone status %d", code)
	}
	payload := fmt.Sprintf(`{"name":"version-one-1","zone_type":"operating","polygon_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"min_height_mm":0,"max_height_mm":1000,"speed_limit_mm_s":100,"access_rule":"none","version":1}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/zones/%d", id), bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+engToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update zone with version 1 status = %d, want 200", w.Code)
	}
}

func TestZoneWriteRequiresRole(t *testing.T) {
	engine, engToken, audToken := r001Router(t)
	code, id := r001CreateZone(t, engine, engToken, "operating", "role-update-1")
	if code != http.StatusCreated {
		t.Fatalf("create zone status %d", code)
	}
	payload := fmt.Sprintf(`{"name":"role-update-1","zone_type":"operating","polygon_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"min_height_mm":0,"max_height_mm":1000,"speed_limit_mm_s":100,"access_rule":"none","version":1}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/zones/%d", id), bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+audToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("auditor updating a zone status = %d, want 403", w.Code)
	}
}

func r001ZoneTransition(t *testing.T, engine *gin.Engine, token, action string, id uint, version int) int {
	t.Helper()
	payload := fmt.Sprintf(`{"version":%d}`, version)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/zones/%d/%s", id, action), bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code
}

func TestZoneActivateRequiresRole(t *testing.T) {
	engine, engToken, audToken := r001Router(t)
	code, id := r001CreateZone(t, engine, engToken, "operating", "role-activate-1")
	if code != http.StatusCreated {
		t.Fatalf("create zone status %d", code)
	}
	if code := r001ZoneTransition(t, engine, audToken, "activate", id, 1); code != http.StatusForbidden {
		t.Fatalf("auditor activating a zone status = %d, want 403", code)
	}
}

func TestZoneDeactivateRequiresRole(t *testing.T) {
	engine, engToken, audToken := r001Router(t)
	code, id := r001CreateZone(t, engine, engToken, "operating", "role-deactivate-1")
	if code != http.StatusCreated {
		t.Fatalf("create zone status %d", code)
	}
	if code := r001ZoneTransition(t, engine, engToken, "activate", id, 1); code != http.StatusOK {
		t.Fatalf("engineer activate status %d", code)
	}
	code = r001ZoneTransition(t, engine, audToken, "deactivate", id, 2)
	if code != http.StatusForbidden {
		t.Fatalf("auditor deactivating a zone status = %d, want 403", code)
	}
}
