package domain

import (
	"encoding/json"
	"testing"
)

func TestGeofenceEvent_JSONShape(t *testing.T) {
	evt := GeofenceEvent{
		VehicleID:  "B1234XYZ",
		Event:      "geofence_entry",
		Location:   GeoPoint{Latitude: -6.1754, Longitude: 106.8272},
		Timestamp:  1715003456,
		GeofenceID: "monas", // internal-only, must NOT appear in JSON
	}
	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify GeofenceID is excluded (json:"-").
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["geofence_id"]; ok {
		t.Error("geofence_id leaked to JSON")
	}
	if raw["vehicle_id"] != "B1234XYZ" {
		t.Errorf("vehicle_id mismatch: %v", raw["vehicle_id"])
	}
	if raw["event"] != "geofence_entry" {
		t.Errorf("event mismatch: %v", raw["event"])
	}

	// Verify nested location object shape.
	locRaw, ok := raw["location"].(map[string]any)
	if !ok {
		t.Fatalf("location missing or wrong type: %T", raw["location"])
	}
	if locRaw["latitude"] != -6.1754 {
		t.Errorf("location.latitude mismatch: %v", locRaw["latitude"])
	}
	if locRaw["longitude"] != 106.8272 {
		t.Errorf("location.longitude mismatch: %v", locRaw["longitude"])
	}

	// Verify timestamp is numeric.
	if raw["timestamp"] != float64(1715003456) {
		t.Errorf("timestamp mismatch: %v", raw["timestamp"])
	}
}
