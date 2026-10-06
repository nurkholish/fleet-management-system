package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
	"fleet-management-system/internal/usecase"
)

// mockLocationRepo is an in-memory LocationRepository for router tests.
type mockLocationRepo struct {
	latest domain.Location
	hist   []domain.Location
	err    error
}

func (m *mockLocationRepo) Save(_ context.Context, _ domain.Location) error {
	return m.err
}

func (m *mockLocationRepo) GetLatest(_ context.Context, _ string) (domain.Location, error) {
	if m.err != nil {
		return domain.Location{}, m.err
	}
	return m.latest, nil
}

func (m *mockLocationRepo) GetHistory(
	_ context.Context, _ string, _, _, _ int64, _ int,
) ([]domain.Location, int64, bool, error) {
	if m.err != nil {
		return nil, 0, false, m.err
	}
	return m.hist, 0, false, nil
}

func newLocationUC(repo domain.LocationRepository) *usecase.LocationUsecase {
	return usecase.NewLocationUsecase(repo, 7*24*3600, 1000, 10000)
}

// buildRouter wires all handlers required by NewRouter.
func buildRouter(repo *mockLocationRepo) *gin.Engine {
	lh := NewLocationHandler(newLocationUC(repo), zerolog.Nop())
	hh := NewHealthHandler(&mockPinger{}, &mockHealthChecker{healthy: true})
	gh := NewGeofenceHandler(nil, zerolog.Nop())
	return NewRouter(lh, hh, gh, zerolog.Nop(), "test")
}

func TestNewRouter_RoutesRegistered(t *testing.T) {
	repo := &mockLocationRepo{
		latest: domain.Location{
			VehicleID: "B1234XYZ",
			Latitude:  -6.2,
			Longitude: 106.8,
			Timestamp: 1715003456,
		},
	}
	r := buildRouter(repo)

	// /healthz
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/healthz: expected 200, got %d", w.Code)
	}

	// /readyz
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/readyz: expected 200, got %d", w.Code)
	}

	// /geofences
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/geofences", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/geofences: expected 200, got %d", w.Code)
	}

	// /vehicles/:id/location
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/vehicles/B1234XYZ/location", nil))
	if w.Code == http.StatusNotFound {
		t.Errorf("/vehicles/:id/location: route not registered")
	}

	// /vehicles/:id/history
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/vehicles/B1234XYZ/history?start=100&end=200", nil))
	if w.Code == http.StatusNotFound {
		t.Errorf("/vehicles/:id/history: route not registered")
	}
}

func TestNewRouter_SecurityHeaders(t *testing.T) {
	r := buildRouter(&mockLocationRepo{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options header")
	}
	if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("missing X-Frame-Options header")
	}
}
