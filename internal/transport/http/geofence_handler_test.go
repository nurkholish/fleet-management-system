package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
)

func setupGeofenceRouter(fences []domain.Geofence) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewGeofenceHandler(fences, zerolog.Nop())
	r := gin.New()
	r.GET("/geofences", h.List)
	return r
}

func TestGeofenceHandler_List_WithFences(t *testing.T) {
	fences := []domain.Geofence{
		{ID: "monas", Name: "Monas", Latitude: -6.1754, Longitude: 106.8272, RadiusMeters: 50},
		{ID: "blok_m", Name: "Blok M", Latitude: -6.2444, Longitude: 106.8006, RadiusMeters: 50},
	}
	r := setupGeofenceRouter(fences)

	req := httptest.NewRequest(http.MethodGet, "/geofences", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var got []domain.Geofence
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 fences, got %d", len(got))
	}
	if got[0].ID != "monas" {
		t.Errorf("fence[0].id=%q, want monas", got[0].ID)
	}
	if got[0].RadiusMeters != 50 {
		t.Errorf("fence[0].radius=%v, want 50", got[0].RadiusMeters)
	}
}

func TestGeofenceHandler_List_Empty(t *testing.T) {
	r := setupGeofenceRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/geofences", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body != "null" && body != "[]" {
		t.Errorf("expected null or [], got %q", body)
	}
}
