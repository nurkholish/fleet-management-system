package mqtt

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// validTopic is the canonical format that ParseLocation cross-validates against.
const validTopic = "/fleet/vehicle/B1234XYZ/location"

// mkPayload builds a valid JSON payload with the given vehicle_id.
// extraFields is a raw JSON fragment (without leading comma), or "" for none.
// Timestamp uses current time so it passes the domain's freshness check.
func mkPayload(vehicleID string, extraFields string) []byte {
	now := time.Now().Unix()
	body := `{"vehicle_id":"` + vehicleID + `",` +
		`"latitude":-6.2088,` +
		`"longitude":106.8456,` +
		`"timestamp":` + strconv.FormatInt(now, 10)
	if extraFields != "" {
		body += "," + extraFields
	}
	body += `}`
	return []byte(body)
}

// ============================================================
// ParseLocation — happy path
// ============================================================

func TestParseLocation_Valid(t *testing.T) {
	loc, err := ParseLocation(validTopic, mkPayload("B1234XYZ", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc.VehicleID != "B1234XYZ" {
		t.Errorf("vehicle=%q, want B1234XYZ", loc.VehicleID)
	}
	if loc.Latitude != -6.2088 {
		t.Errorf("lat=%f, want -6.2088", loc.Latitude)
	}
	if loc.Longitude != 106.8456 {
		t.Errorf("lon=%f, want 106.8456", loc.Longitude)
	}
	if loc.Timestamp <= 0 {
		t.Errorf("timestamp should be positive, got %d", loc.Timestamp)
	}
}

// ============================================================
// ParseLocation — topic / payload cross-validation
// ============================================================

func TestParseLocation_VehicleIDMismatch(t *testing.T) {
	// Topic says B1234XYZ, payload says B9999XYZ → must reject.
	_, err := ParseLocation(validTopic, mkPayload("B9999XYZ", ""))
	if err == nil {
		t.Fatal("expected error for topic/payload vehicle_id mismatch")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error should mention mismatch, got: %v", err)
	}
}

func TestParseLocation_TopicWithoutVehicleID(t *testing.T) {
	// Malformed topic → extractVehicleID returns "" → no cross-check.
	// Payload must still validate on its own.
	loc, err := ParseLocation("/random/topic", mkPayload("B1234XYZ", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc.VehicleID != "B1234XYZ" {
		t.Errorf("vehicle=%q, want B1234XYZ", loc.VehicleID)
	}
}

// ============================================================
// ParseLocation — payload validation
// ============================================================

func TestParseLocation_Empty(t *testing.T) {
	if _, err := ParseLocation(validTopic, nil); err == nil {
		t.Fatal("expected error for nil payload")
	}
	if _, err := ParseLocation(validTopic, []byte{}); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestParseLocation_TooLarge(t *testing.T) {
	// > 1024 bytes
	big := []byte(`{"vehicle_id":"` + strings.Repeat("A", 1100) + `"}`)
	if _, err := ParseLocation(validTopic, big); err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestParseLocation_InvalidJSON(t *testing.T) {
	if _, err := ParseLocation(validTopic, []byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseLocation_UnknownField(t *testing.T) {
	_, err := ParseLocation(validTopic, mkPayload("B1234XYZ", `"extra":"rejected"`))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseLocation_TrailingData(t *testing.T) {
	now := time.Now().Unix()
	payload := []byte(`{"vehicle_id":"B1234XYZ","latitude":-6.2,"longitude":106.8,"timestamp":` +
		strconv.FormatInt(now, 10) + `}{}`)
	if _, err := ParseLocation(validTopic, payload); err == nil {
		t.Fatal("expected error for trailing data")
	}
}

func TestParseLocation_InvalidPlate(t *testing.T) {
	_, err := ParseLocation(validTopic, mkPayload("invalid", ""))
	if err == nil {
		t.Fatal("expected validation error for invalid plate")
	}
}

func TestParseLocation_LatitudeOutOfRange(t *testing.T) {
	now := time.Now().Unix()
	payload := []byte(`{"vehicle_id":"B1234XYZ","latitude":-100,"longitude":106.8,"timestamp":` +
		strconv.FormatInt(now, 10) + `}`)
	if _, err := ParseLocation(validTopic, payload); err == nil {
		t.Fatal("expected validation error for latitude out of range")
	}
}

func TestParseLocation_LongitudeOutOfRange(t *testing.T) {
	now := time.Now().Unix()
	payload := []byte(`{"vehicle_id":"B1234XYZ","latitude":-6.2,"longitude":200,"timestamp":` +
		strconv.FormatInt(now, 10) + `}`)
	if _, err := ParseLocation(validTopic, payload); err == nil {
		t.Fatal("expected validation error for longitude out of range")
	}
}

func TestParseLocation_ZeroTimestamp(t *testing.T) {
	payload := []byte(`{"vehicle_id":"B1234XYZ","latitude":-6.2,"longitude":106.8,"timestamp":0}`)
	if _, err := ParseLocation(validTopic, payload); err == nil {
		t.Fatal("expected validation error for zero timestamp")
	}
}

// ============================================================
// extractVehicleIDFromTopic — table-driven
// ============================================================

func TestExtractVehicleIDFromTopic(t *testing.T) {
	cases := []struct {
		name  string
		topic string
		want  string
	}{
		{"valid monas", "/fleet/vehicle/B1234XYZ/location", "B1234XYZ"},
		{"valid 2", "/fleet/vehicle/B5678ABC/location", "B5678ABC"},
		{"empty id", "/fleet/vehicle//location", ""},
		{"extra segment", "/fleet/vehicle/B123/location/extra", ""},
		{"wrong prefix", "/other/vehicle/B1234XYZ/location", ""},
		{"missing suffix", "/fleet/vehicle/B1234XYZ", ""},
		{"empty string", "", ""},
		{"only slashes", "///", ""},
		{"trailing slash", "/fleet/vehicle/B1234XYZ/location/", ""},
		{"wrong topic level", "/fleet/B1234XYZ/location", ""},
		{"wrong prefix fleet typo", "/fleets/vehicle/B1234XYZ/location", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractVehicleIDFromTopic(tc.topic)
			if got != tc.want {
				t.Errorf("topic=%q got=%q want=%q", tc.topic, got, tc.want)
			}
		})
	}
}

// ============================================================
// Benchmarks (optional, run with -bench=. -benchmem)
// ============================================================

func BenchmarkParseLocation_Valid(b *testing.B) {
	payload := mkPayload("B1234XYZ", "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseLocation(validTopic, payload)
	}
}

func BenchmarkExtractVehicleIDFromTopic(b *testing.B) {
	topic := "/fleet/vehicle/B1234XYZ/location"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractVehicleIDFromTopic(topic)
	}
}
