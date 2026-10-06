package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
	"fleet-management-system/internal/usecase"
)

// mockLocationRepoFull is a richer mock for handler tests.
// Named distinctly so it doesn't clash with mockLocationRepo in router_test.go.
type mockLocationRepoFull struct {
	latest     domain.Location
	latestErr  error
	history    []domain.Location
	nextCursor int64
	hasMore    bool
	histErr    error
	saveErr    error
}

func (m *mockLocationRepoFull) Save(_ context.Context, _ domain.Location) error {
	return m.saveErr
}

func (m *mockLocationRepoFull) GetLatest(_ context.Context, _ string) (domain.Location, error) {
	return m.latest, m.latestErr
}

func (m *mockLocationRepoFull) GetHistory(
	_ context.Context, _ string, _, _, _ int64, _ int,
) ([]domain.Location, int64, bool, error) {
	return m.history, m.nextCursor, m.hasMore, m.histErr
}

func setupLocationRouter(repo domain.LocationRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	uc := usecase.NewLocationUsecase(repo, 604800, 1000, 10000)
	h := NewLocationHandler(uc, zerolog.Nop())

	r := gin.New()
	r.GET("/vehicles/:vehicle_id/location", h.GetLatest)
	r.GET("/vehicles/:vehicle_id/history", h.GetHistory)
	return r
}

func TestGetLatest_Success(t *testing.T) {
	repo := &mockLocationRepoFull{
		latest: domain.Location{
			VehicleID: "B1234XYZ",
			Latitude:  -6.2088,
			Longitude: 106.8456,
			Timestamp: 1715003456,
		},
	}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/vehicles/B1234XYZ/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var body domain.Location
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.VehicleID != "B1234XYZ" {
		t.Errorf("vehicle mismatch: %s", body.VehicleID)
	}
	if body.Latitude != -6.2088 {
		t.Errorf("lat mismatch: %f", body.Latitude)
	}
	if body.Timestamp != 1715003456 {
		t.Errorf("ts mismatch: %d", body.Timestamp)
	}
}

func TestGetLatest_NotFound(t *testing.T) {
	repo := &mockLocationRepoFull{latestErr: domain.ErrNotFound}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/vehicles/UNKNOWN/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetLatest_ValidationError(t *testing.T) {
	repo := &mockLocationRepoFull{latestErr: domain.ErrValidation}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/vehicles/B1234XYZ/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetLatest_InternalError_NoLeak(t *testing.T) {
	repo := &mockLocationRepoFull{latestErr: errors.New("db down: connection refused at 10.0.0.5:5432")}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/vehicles/B1234XYZ/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] != "internal error" {
		t.Errorf("expected generic 'internal error', got: %q", body["error"])
	}
}

func TestGetHistory_Success(t *testing.T) {
	repo := &mockLocationRepoFull{
		history: []domain.Location{
			{VehicleID: "B1234XYZ", Latitude: -6.2, Longitude: 106.8, Timestamp: 1715000000},
			{VehicleID: "B1234XYZ", Latitude: -6.21, Longitude: 106.81, Timestamp: 1715000060},
		},
		nextCursor: 0,
		hasMore:    false,
	}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=1714999000&end=1715009999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var body struct {
		VehicleID  string            `json:"vehicle_id"`
		Count      int               `json:"count"`
		HasMore    bool              `json:"has_more"`
		NextCursor int64             `json:"next_cursor"`
		Data       []domain.Location `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Count != 2 {
		t.Errorf("expected count=2, got %d", body.Count)
	}
	if body.VehicleID != "B1234XYZ" {
		t.Errorf("vehicle mismatch: %s", body.VehicleID)
	}
	if len(body.Data) != 2 {
		t.Errorf("expected 2 rows, got %d", len(body.Data))
	}
	if body.HasMore {
		t.Errorf("expected has_more=false")
	}
}

func TestGetHistory_WithNextCursor(t *testing.T) {
	repo := &mockLocationRepoFull{
		history: []domain.Location{
			{VehicleID: "B1234XYZ", Timestamp: 1715000000},
		},
		nextCursor: 1715000001,
		hasMore:    true,
	}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=1714999000&end=1715009999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body struct {
		HasMore    bool  `json:"has_more"`
		NextCursor int64 `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !body.HasMore {
		t.Errorf("expected has_more=true")
	}
	if body.NextCursor != 1715000001 {
		t.Errorf("expected next_cursor=1715000001, got %d", body.NextCursor)
	}
}

func TestGetHistory_MissingParams(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/vehicles/B1234XYZ/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetHistory_NonIntegerParams(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=abc&end=def", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetHistory_InvalidRange(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=100&end=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetHistory_RangeExceedsMax(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=1&end=31536000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetHistory_LimitClamped(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=100&end=200&limit=999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetHistory_RepositoryError_NoLeak(t *testing.T) {
	repo := &mockLocationRepoFull{histErr: errors.New("pq: relation does not exist")}
	r := setupLocationRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=100&end=200", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] != "internal error" {
		t.Errorf("error leaked: %q", body["error"])
	}
}

// TestGetHistory_StartNonInteger covers the "start must be a valid unix
// timestamp" branch specifically (end is valid so code reaches start parse).
func TestGetHistory_StartNonInteger(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	// start=abc (invalid), end=200 (valid) → error at start parse.
	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=abc&end=200", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] != "start must be a valid unix timestamp" {
		t.Errorf("unexpected error: %q", body["error"])
	}
}

// TestGetHistory_EndNonInteger covers the "end must be a valid unix
// timestamp" branch (start is valid, so code reaches end parse).
func TestGetHistory_EndNonInteger(t *testing.T) {
	repo := &mockLocationRepoFull{}
	r := setupLocationRouter(repo)

	// start=100 (valid), end=abc (invalid) → error at end parse.
	req := httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=100&end=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] != "end must be a valid unix timestamp" {
		t.Errorf("unexpected error: %q", body["error"])
	}
}
