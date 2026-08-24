package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/config"
	"robot-cell-safety-envelope-validator/backend/internal/router"
)

func r008Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r008-test-secret-0123456789abcdef",
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
	body := bytes.NewBufferString(`{"username":"programmer","password":"Program#533"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d", w.Code)
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

func TestCreateMotionProgramRejectsSinglePointTrajectory(t *testing.T) {
	engine, token := r008Router(t)
	payload := `{"robot_cell_id":1,"program_code":"SINGLE-PT-08","version":1,"trajectory":[{"x_mm":0,"y_mm":0,"z_mm":0,"time_ms":0,"speed_mm_s":10}],"tool_radius_mm":100,"payload_radius_mm":50,"interlock_sequence":[{"name":"estop","sequence":1,"depends_on":[]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/programs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("single-point trajectory program status = %d, want 400", w.Code)
	}
}
