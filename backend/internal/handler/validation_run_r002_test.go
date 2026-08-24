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

func r002Router(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r002-test-secret-0123456789abcdef",
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

func TestListValidationRunsStatusFilter(t *testing.T) {
	engine, engToken, _ := r002Router(t)
	// 创建一条新的校验（含碰撞 -> failed），与种子的 reviewed 记录区分
	body := bytes.NewBufferString(`{"motion_program_id": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/validations", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+engToken)
	req.Header.Set("Idempotency-Key", "r002-list-filter-key-1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create validation status %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/validations?status=reviewed", nil)
	req.Header.Set("Authorization", "Bearer "+engToken)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d", w.Code)
	}
	var resp struct {
		Data []struct {
			ValidationStatus string `json:"validation_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected at least the seeded reviewed run")
	}
	for _, item := range resp.Data {
		if item.ValidationStatus != "reviewed" {
			t.Fatalf("list?status=reviewed returned status %q", item.ValidationStatus)
		}
	}
}

func TestValidationReviewRequiresRole(t *testing.T) {
	engine, _, audToken := r002Router(t)
	// 种子校验记录 id=1 (reviewed)
	body := bytes.NewBufferString(`{"note":"auditor tries to void"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/validations/1/void", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+audToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("auditor voiding a validation status = %d, want 403", w.Code)
	}
}
