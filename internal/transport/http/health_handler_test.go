package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// mockPinger implements the pinger interface.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

// mockHealthChecker implements the healthChecker interface.
type mockHealthChecker struct {
	healthy bool
}

func (m *mockHealthChecker) IsHealthy() bool { return m.healthy }

func setupHealthRouter(pg pinger, rabbit healthChecker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHealthHandler(pg, rabbit)
	r := gin.New()
	r.GET("/healthz", h.Live)
	r.GET("/readyz", h.Ready)
	return r
}

func TestHealthLive(t *testing.T) {
	r := setupHealthRouter(&mockPinger{}, &mockHealthChecker{healthy: true})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
}

func TestHealthReady_OK(t *testing.T) {
	r := setupHealthRouter(&mockPinger{}, &mockHealthChecker{healthy: true})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("expected ready, got %q", body["status"])
	}
}

func TestHealthReady_PGDown(t *testing.T) {
	r := setupHealthRouter(
		&mockPinger{err: errors.New("connection refused")},
		&mockHealthChecker{healthy: true},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "degraded" {
		t.Errorf("expected degraded, got %q", body["status"])
	}
	if body["postgres"] == "" {
		t.Error("expected postgres error message")
	}
}

func TestHealthReady_RabbitUnhealthy(t *testing.T) {
	r := setupHealthRouter(&mockPinger{}, &mockHealthChecker{healthy: false})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["rabbitmq"] != "unhealthy" {
		t.Errorf("expected rabbitmq=unhealthy, got %q", body["rabbitmq"])
	}
}
