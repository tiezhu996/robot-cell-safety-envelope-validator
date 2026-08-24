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
	"robot-cell-safety-envelope-validator/backend/internal/model"
	"robot-cell-safety-envelope-validator/backend/internal/router"
)

func r010Setup(t *testing.T) (*gin.Engine, string, string, *gorm.DB) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r010-test-secret-0123456789abcdef",
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
	return engine, login("engineer", "Safety#533"), login("programmer", "Program#533"), db
}

func r010CreateProgram(t *testing.T, engine *gin.Engine, token string, cellID uint, code string) uint {
	t.Helper()
	payload := fmt.Sprintf(`{"robot_cell_id":%d,"program_code":"%s","version":1,"trajectory":[{"x_mm":0,"y_mm":0,"z_mm":0,"time_ms":0,"speed_mm_s":10},{"x_mm":100,"y_mm":0,"z_mm":0,"time_ms":1000,"speed_mm_s":10}],"tool_radius_mm":100,"payload_radius_mm":50,"interlock_sequence":[{"name":"estop","sequence":1,"depends_on":[]}]}`, cellID, code)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/programs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create program status %d", w.Code)
	}
	var resp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode program: %v", err)
	}
	return resp.Data.ID
}

func r010Transition(t *testing.T, engine *gin.Engine, token string, id uint, target string) int {
	t.Helper()
	body := bytes.NewBufferString(fmt.Sprintf(`{"target_state":"%s"}`, target))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/programs/%d/transition", id), body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code
}

func TestTransitionProgramRejectsIllegalState(t *testing.T) {
	engine, _, progToken, _ := r010Setup(t)
	id := r010CreateProgram(t, engine, progToken, 1, "ILLEGAL-R10")
	if code := r010Transition(t, engine, progToken, id, "active"); code != http.StatusConflict {
		t.Fatalf("illegal uploaded->active transition status = %d, want 409", code)
	}
}

func TestTransitionProgramParseFailureRejects(t *testing.T) {
	engine, _, progToken, db := r010Setup(t)
	id := r010CreateProgram(t, engine, progToken, 1, "PARSE-R10")
	if err := db.Model(&model.MotionProgram{}).Where("id = ?", id).Update("trajectory_json", `not-json`).Error; err != nil {
		t.Fatalf("corrupt trajectory: %v", err)
	}
	if code := r010Transition(t, engine, progToken, id, "parsed"); code != http.StatusUnprocessableEntity {
		t.Fatalf("parse-failure transition status = %d, want 422", code)
	}
	var program model.MotionProgram
	if err := db.First(&program, id).Error; err != nil {
		t.Fatalf("reload program: %v", err)
	}
	if program.ProgramState != "rejected" {
		t.Fatalf("program state after parse failure = %q, want rejected", program.ProgramState)
	}
}

func TestTransitionProgramActiveSupersedesSameCellOnly(t *testing.T) {
	engine, engToken, progToken, _ := r010Setup(t)
	// 创建 cell B（engineer）
	payload := `{"cell_code":"CELL-R10B","name":"Cell B","layout_geojson":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]},"robot_model":"R","controller_model":"C","max_reach_mm":2000,"owner_team":"T"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+engToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create cell status %d", w.Code)
	}
	var cellResp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cellResp); err != nil {
		t.Fatalf("decode cell: %v", err)
	}
	cellB := cellResp.Data.ID

	p1 := r010CreateProgram(t, engine, progToken, 1, "SUP-A-R10")
	for _, target := range []string{"parsed", "ready", "active"} {
		if code := r010Transition(t, engine, progToken, p1, target); code != http.StatusOK {
			t.Fatalf("P1 to %s status %d", target, code)
		}
	}
	p3 := r010CreateProgram(t, engine, progToken, cellB, "SUP-B-R10")
	for _, target := range []string{"parsed", "ready", "active"} {
		if code := r010Transition(t, engine, progToken, p3, target); code != http.StatusOK {
			t.Fatalf("P3 to %s status %d", target, code)
		}
	}
	getState := func(id uint) string {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/programs/%d", id), nil)
		req.Header.Set("Authorization", "Bearer "+progToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		var resp struct {
			Data struct {
				ProgramState string `json:"program_state"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return resp.Data.ProgramState
	}
	if state := getState(p1); state != "active" {
		t.Fatalf("P1 (cell A) state after P3 (cell B) activation = %q, want active", state)
	}
	p2 := r010CreateProgram(t, engine, progToken, 1, "SUP-A2-R10")
	for _, target := range []string{"parsed", "ready", "active"} {
		if code := r010Transition(t, engine, progToken, p2, target); code != http.StatusOK {
			t.Fatalf("P2 to %s status %d", target, code)
		}
	}
	if state := getState(p1); state != "superseded" {
		t.Fatalf("P1 state after P2 (same cell) activation = %q, want superseded", state)
	}
	if state := getState(p3); state != "active" {
		t.Fatalf("P3 (cell B) state = %q, want active", state)
	}
}
