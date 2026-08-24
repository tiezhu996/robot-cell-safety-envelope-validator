package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/config"
	"robot-cell-safety-envelope-validator/backend/internal/router"
)

func r007Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r007-test-secret-0123456789abcdef",
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
	body := bytes.NewBufferString(`{"username":"auditor","password":"Audit#533"}`)
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

func TestRecoveryWritesErrorResponse(t *testing.T) {
	engine, token := r007Router(t)
	engine.GET("/panic-r007", func(c *gin.Context) {
		panic("r007 deliberate panic")
	})
	req := httptest.NewRequest(http.MethodGet, "/panic-r007", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic route status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatalf("panic route response missing error body: %s", w.Body.String())
	}
}

func TestErrorHandlerWritesErrorResponse(t *testing.T) {
	engine, token := r007Router(t)
	engine.GET("/err-r007", func(c *gin.Context) {
		_ = c.Error(errors.New("r007 unhandled detail"))
	})
	req := httptest.NewRequest(http.MethodGet, "/err-r007", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("unhandled error route status = %d, want 500", w.Code)
	}
}

func TestRBACForbiddenResponse(t *testing.T) {
	engine, token := r007Router(t)
	payload := `{"cell_code":"CELL-R7A","name":"Cell R7","layout_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"robot_model":"R","controller_model":"C","max_reach_mm":2000,"owner_team":"Team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("auditor creating a cell status = %d, want 403 (RBAC must reject)", w.Code)
	}
	// 关键：被拒绝的操作绝不能真的执行
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cells", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list cells status %d", w.Code)
	}
	var listResp struct {
		Data []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode cells: %v", err)
	}
	for _, item := range listResp.Data {
		if item.Name == "Cell R7" {
			t.Fatalf("forbidden cell creation actually executed (RBAC bypass): %+v", item)
		}
	}
}
