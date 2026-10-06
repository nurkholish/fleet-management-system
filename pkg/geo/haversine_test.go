package geo

import (
	"math"
	"testing"
)

func TestHaversine_SamePoint(t *testing.T) {
	d := HaversineMeters(-6.2, 106.8, -6.2, 106.8)
	if d != 0 {
		t.Errorf("expected 0, got %f", d)
	}
}

// FIX: Monas → Blok M is actually ~8.2km, not 6.9km.
// Verified via independent calculation:
//
//	ΔLat 0.069° × 111km/° ≈ 7.67km
//	ΔLon 0.0266° × 111km/° × cos(6.2°) ≈ 2.95km
//	hypot(7.67, 2.95) ≈ 8.22km
func TestHaversine_KnownDistance(t *testing.T) {
	d := HaversineMeters(-6.1754, 106.8272, -6.2444, 106.8006)
	if d < 8000 || d > 8400 {
		t.Errorf("expected ~8.2km, got %f", d)
	}
}

func TestHaversine_SmallDistance(t *testing.T) {
	// ~11 meters north.
	d := HaversineMeters(-6.2, 106.8, -6.2001, 106.8)
	if d < 5 || d > 20 {
		t.Errorf("expected ~11m, got %f", d)
	}
}

func TestHaversine_Antipodal(t *testing.T) {
	d := HaversineMeters(0, 0, 0, 180)
	want := math.Pi * earthRadiusM
	if math.Abs(d-want) > 1 {
		t.Errorf("expected %f, got %f", want, d)
	}
}
