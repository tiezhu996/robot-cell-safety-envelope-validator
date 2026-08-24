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

func r009Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r009-test-secret-0123456789abcdef",
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

func r009CreateProgram(t *testing.T, engine *gin.Engine, token, code string, version int) int {
	t.Helper()
	payload := fmt.Sprintf(`{"robot_cell_id":1,"program_code":"%s","version":%d,"trajectory":[{"x_mm":0,"y_mm":0,"z_mm":0,"time_ms":0,"speed_mm_s":10},{"x_mm":100,"y_mm":0,"z_mm":0,"time_ms":1000,"speed_mm_s":10}],"tool_radius_mm":100,"payload_radius_mm":50,"interlock_sequence":[{"name":"estop","sequence":1,"depends_on":[]}]}`, code, version)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/programs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code
}

func TestCreateMotionProgramDuplicateReturns409(t *testing.T) {
	engine, token := r009Router(t)
	first := r009CreateProgram(t, engine, token, "DUP-TEST-01", 1)
	if first != http.StatusCreated {
		t.Fatalf("first create status %d, want 201", first)
	}
	second := r009CreateProgram(t, engine, token, "DUP-TEST-01", 1)
	if second != http.StatusConflict {
		t.Fatalf("duplicate program create status = %d, want 409", second)
	}
}

func TestCreateMotionProgramInvalidBodyReturns400(t *testing.T) {
	engine, token := r009Router(t)
	payload := `{"robot_cell_id":1}` // 缺 trajectory / interlock
	req := httptest.NewRequest(http.MethodPost, "/api/v1/programs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid body status = %d, want 400", w.Code)
	}
}

func TestTransitionMotionProgramConflictReturns409(t *testing.T) {
	engine, token := r009Router(t)
	status := r009CreateProgram(t, engine, token, "TRANS-CONFLICT-01", 1)
	if status != http.StatusCreated {
		t.Fatalf("create status %d, want 201", status)
	}
	// 找到刚创建的程序 id
	req := httptest.NewRequest(http.MethodGet, "/api/v1/programs?state=uploaded", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list programs status %d", w.Code)
	}
	var listResp struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode programs: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatal("no uploaded program found")
	}
	// uploaded -> active 是非法迁移，应 409
	body := bytes.NewBufferString(`{"target_state":"active"}`)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/programs/%d/transition", listResp.Data[0].ID), body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("illegal program transition status = %d, want 409", w.Code)
	}
}

func TestGetMotionProgramMissingReturns404(t *testing.T) {
	engine, token := r009Router(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/programs/999999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get missing program status = %d, want 404", w.Code)
	}
}

func TestTransitionMotionProgramInvalidBodyReturns400(t *testing.T) {
	engine, token := r009Router(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/programs/1/transition", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid transition body status = %d, want 400", w.Code)
	}
}
