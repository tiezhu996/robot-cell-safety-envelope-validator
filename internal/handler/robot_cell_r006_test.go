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

func r006Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r006-test-secret-0123456789abcdef",
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
		t.Fatalf("login status %d: %s", w.Code, w.Body.String())
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return engine, loginResp.Data.Token
}

func r006CreateCell(t *testing.T, engine *gin.Engine, token, code string) uint {
	t.Helper()
	payload := fmt.Sprintf(`{"cell_code":"%s","name":"Cell %s","layout_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"robot_model":"R","controller_model":"C","max_reach_mm":2000,"owner_team":"Team"}`, code, code)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create cell status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode cell: %v", err)
	}
	return resp.Data.ID
}

func r006CellAction(t *testing.T, engine *gin.Engine, token, action string, id uint) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/cells/%d/%s", id, action), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code
}

func r006UpdateCell(t *testing.T, engine *gin.Engine, token string, id uint, version int) int {
	t.Helper()
	payload := fmt.Sprintf(`{"name":"Renamed","layout_geojson":{"type":"Polygon","coordinates":[[[0,0],[200,0],[200,200],[0,200],[0,0]]]},"robot_model":"R2","controller_model":"C2","max_reach_mm":2500,"owner_team":"Team2","layout_version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/cells/%d", id), bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code
}

func TestRobotCellFrozenRejectsLayoutEdit(t *testing.T) {
	engine, token := r006Router(t)
	id := r006CreateCell(t, engine, token, "CELL-R6F")
	if code := r006CellAction(t, engine, token, "freeze", id); code != http.StatusOK {
		t.Fatalf("freeze status %d", code)
	}
	if code := r006UpdateCell(t, engine, token, id, 1); code != http.StatusConflict {
		t.Fatalf("updating a frozen cell status = %d, want 409", code)
	}
}

func TestRobotCellInactiveCannotFreeze(t *testing.T) {
	engine, token := r006Router(t)
	id := r006CreateCell(t, engine, token, "CELL-R6I")
	if code := r006CellAction(t, engine, token, "freeze", id); code != http.StatusOK {
		t.Fatalf("freeze status %d", code)
	}
	if code := r006CellAction(t, engine, token, "deactivate", id); code != http.StatusOK {
		t.Fatalf("deactivate status %d", code)
	}
	if code := r006CellAction(t, engine, token, "freeze", id); code != http.StatusConflict {
		t.Fatalf("freezing an inactive cell status = %d, want 409", code)
	}
}

func TestRobotCellDoubleDeactivateRejected(t *testing.T) {
	engine, token := r006Router(t)
	id := r006CreateCell(t, engine, token, "CELL-R6D")
	if code := r006CellAction(t, engine, token, "deactivate", id); code != http.StatusOK {
		t.Fatalf("first deactivate status %d", code)
	}
	if code := r006CellAction(t, engine, token, "deactivate", id); code != http.StatusConflict {
		t.Fatalf("second deactivate status = %d, want 409", code)
	}
}
