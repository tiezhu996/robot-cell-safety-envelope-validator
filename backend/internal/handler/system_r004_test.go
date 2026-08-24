package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/config"
	"robot-cell-safety-envelope-validator/backend/internal/handler"
	"robot-cell-safety-envelope-validator/backend/internal/repository"
	"robot-cell-safety-envelope-validator/backend/internal/router"
	"robot-cell-safety-envelope-validator/backend/internal/service"
)

func r004Cfg(name string) config.Config {
	return config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + name + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r004-test-secret-0123456789abcdef",
		JWTTTL:             2 * time.Hour,
		CORSOrigin:         "http://localhost:18533",
		LogLevel:           "info",
		RateLimitPerMinute: 10000,
		AlgorithmVersion:   "envelope-2d-height-v1.0",
	}
}

func TestReadyHonorsRequestCancellation(t *testing.T) {
	cfg := r004Cfg(strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	systemRepo := repository.NewSystemRepository(db)
	systemSvc := service.NewSystemService(systemRepo, cfg.JWTSecret, cfg.JWTTTL)
	h := handler.NewSystemHandler(systemSvc, db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	h.Ready(c)
	if c.Writer.Status() != http.StatusInternalServerError {
		t.Fatalf("Ready with cancelled request context status = %d, want 500", c.Writer.Status())
	}
}

func TestAuthRejectsCancelledRequestContext(t *testing.T) {
	cfg := r004Cfg(strings.ReplaceAll(t.Name(), "/", "_"))
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("audit with cancelled request context status = %d, want 401", w.Code)
	}
}

func TestListAuditHonorsContextCancellation(t *testing.T) {
	cfg := r004Cfg(strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	systemRepo := repository.NewSystemRepository(db)
	systemSvc := service.NewSystemService(systemRepo, cfg.JWTSecret, cfg.JWTTTL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := systemSvc.ListAudit(ctx, 1, 50, "", "", "", "", nil, nil); err == nil {
		t.Fatal("cancelled context must abort audit listing")
	}
}
