package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"robot-cell-safety-envelope-validator/backend/internal/config"
)

func r005Config(name string) config.Config {
	return config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN:              "file:" + name + "?mode=memory&cache=shared",
		DBAutoMigrate:      true,
		JWTSecret:          "r005-test-secret-0123456789abcdef",
		JWTTTL:             2 * time.Hour,
		CORSOrigin:         "http://localhost:18533",
		LogLevel:           "info",
		RateLimitPerMinute: 10000,
		AlgorithmVersion:   "envelope-2d-height-v1.0",
	}
}

func TestRateLimitConcurrentNoRace(t *testing.T) {
	cfg := r005Config(strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	engine := New(db, cfg)
	workers := 16
	per := 40
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			<-start
			for i := 0; i < per; i++ {
				req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
				req.RemoteAddr = "10.2.0." + string(rune('0'+seed%10)) + ":1234"
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
			}
		}(w)
	}
	close(start)
	wg.Wait()
}

func TestRateLimitPerClientIsolation(t *testing.T) {
	cfg := r005Config(strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	engine := New(db, cfg)
	login := func(ip, username string) int {
		body := bytes.NewBufferString(`{"username":"` + username + `","password":"WrongPass#1"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 30; i++ {
		code := login("10.0.0.1:1111", "nosuchuser-a")
		if code == http.StatusTooManyRequests {
			t.Fatalf("client A blocked before reaching the limit")
		}
	}
	code := login("10.0.0.2:2222", "nosuchuser-b")
	if code == http.StatusTooManyRequests {
		t.Fatalf("client B was rate limited by client A's traffic (shared bucket)")
	}
}

func TestHealthzNotRateLimited(t *testing.T) {
	cfg := r005Config(strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	engine := New(db, cfg)
	for _, path := range []string{"/healthz", "/readyz"} {
		for i := 0; i < 8; i++ {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s request %d status = %d, want 200", path, i+1, w.Code)
			}
		}
	}
}
