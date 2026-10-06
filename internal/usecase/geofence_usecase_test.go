package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	memoryadapter "fleet-management-system/internal/adapter/memory"
	"fleet-management-system/internal/domain"
)

// ============================================================
// Mock publisher
// ============================================================

type mockPublisher struct {
	mu      sync.Mutex
	events  []domain.GeofenceEvent
	err     error
	healthy bool
	closed  bool
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{healthy: true}
}

func (m *mockPublisher) PublishGeofenceEntry(_ context.Context, evt domain.GeofenceEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.healthy {
		return errors.New("publisher unhealthy")
	}
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, evt)
	return nil
}

func (m *mockPublisher) IsHealthy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthy
}

func (m *mockPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = false
	m.closed = true
	return nil
}

func (m *mockPublisher) Events() []domain.GeofenceEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.GeofenceEvent, len(m.events))
	copy(out, m.events)
	return out
}

func (m *mockPublisher) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// ============================================================
// Mock state store (to simulate errors)
// ============================================================

type mockStateStore struct {
	mu         sync.Mutex
	inside     map[string]bool
	enterErr   error
	exitErr    error
	enterCalls int
	exitCalls  int
	resetCalls int
}

func newMockStateStore() *mockStateStore {
	return &mockStateStore{inside: make(map[string]bool)}
}

func (m *mockStateStore) EnterIfOutside(_ context.Context, v, f string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enterCalls++
	if m.enterErr != nil {
		return false, m.enterErr
	}
	k := v + "|" + f
	if m.inside[k] {
		return false, nil
	}
	m.inside[k] = true
	return true, nil
}

func (m *mockStateStore) ExitIfInside(_ context.Context, v, f string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exitCalls++
	if m.exitErr != nil {
		return false, m.exitErr
	}
	k := v + "|" + f
	if !m.inside[k] {
		return false, nil
	}
	delete(m.inside, k)
	return true, nil
}

func (m *mockStateStore) Reset(_ context.Context, v, f string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCalls++
	delete(m.inside, v+"|"+f)
}

// ============================================================
// Helpers
// ============================================================

func defaultFences() []domain.Geofence {
	return []domain.Geofence{
		{
			ID:           "monas",
			Name:         "Monas",
			Latitude:     -6.1754,
			Longitude:    106.8272,
			RadiusMeters: 50,
		},
	}
}

func newTestGeofenceUsecase(fences []domain.Geofence, pub domain.EventPublisher) *GeofenceUsecase {
	return NewGeofenceUsecase(fences, pub, memoryadapter.NewGeofenceState(), zerolog.Nop())
}

func newTestGeofenceUsecaseWithState(
	fences []domain.Geofence,
	pub domain.EventPublisher,
	state domain.GeofenceStateStore,
) *GeofenceUsecase {
	return NewGeofenceUsecase(fences, pub, state, zerolog.Nop())
}

func locAt(vehicleID string, lat, lon float64) domain.Location {
	return domain.Location{
		VehicleID: vehicleID,
		Latitude:  lat,
		Longitude: lon,
		Timestamp: time.Now().Unix(),
	}
}

func monasLoc(vehicleID string) domain.Location {
	return locAt(vehicleID, -6.1754, 106.8272)
}

// ============================================================
// Basic publish semantics
// ============================================================

func TestGeofenceUsecase_InsideRadius_Publishes(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	loc := monasLoc("B1234XYZ")
	uc.Evaluate(context.Background(), loc)

	events := pub.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Event != "geofence_entry" {
		t.Errorf("event=%q, want geofence_entry", e.Event)
	}
	if e.VehicleID != loc.VehicleID {
		t.Errorf("vehicle=%q, want %q", e.VehicleID, loc.VehicleID)
	}
	if e.Location.Latitude != loc.Latitude {
		t.Errorf("lat=%f, want %f", e.Location.Latitude, loc.Latitude)
	}
	if e.Location.Longitude != loc.Longitude {
		t.Errorf("lon=%f, want %f", e.Location.Longitude, loc.Longitude)
	}
	if e.GeofenceID != "monas" {
		t.Errorf("geofenceID=%q, want monas", e.GeofenceID)
	}
	if e.Timestamp != loc.Timestamp {
		t.Errorf("ts=%d, want %d", e.Timestamp, loc.Timestamp)
	}
}

func TestGeofenceUsecase_OutsideRadius_NoPublish(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.1850, 106.8300))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events, got %d", n)
	}
}

// ============================================================
// State-based entry semantics
// ============================================================

func TestGeofenceUsecase_RepeatedInside_OnlyOneEvent(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	loc := monasLoc("B1234XYZ")
	uc.Evaluate(context.Background(), loc)
	uc.Evaluate(context.Background(), loc)
	uc.Evaluate(context.Background(), loc)

	if n := len(pub.Events()); n != 1 {
		t.Fatalf("expected 1 event while staying inside, got %d", n)
	}
}

func TestGeofenceUsecase_ExitThenReenter_PublishesAgain(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	inside := monasLoc("B1234XYZ")
	outside := locAt("B1234XYZ", -6.1900, 106.8400)

	uc.Evaluate(context.Background(), inside)
	uc.Evaluate(context.Background(), outside)
	uc.Evaluate(context.Background(), inside)

	if n := len(pub.Events()); n != 2 {
		t.Fatalf("expected 2 events (entry,exit,entry), got %d", n)
	}
}

func TestGeofenceUsecase_NoFlickerOnBoundary(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.17539, 106.82719))
	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.17541, 106.82721))
	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.17538, 106.82718))

	if n := len(pub.Events()); n != 1 {
		t.Fatalf("expected 1 event on jitter inside, got %d", n)
	}
}

// ============================================================
// Multi-vehicle, multi-fence
// ============================================================

func TestGeofenceUsecase_DifferentVehicles_BothPublish(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	for _, id := range []string{"B1234XYZ", "B5678ABC"} {
		uc.Evaluate(context.Background(), monasLoc(id))
	}

	if n := len(pub.Events()); n != 2 {
		t.Fatalf("expected 2 events, got %d", n)
	}
}

func TestGeofenceUsecase_MultipleFences_BothPublish(t *testing.T) {
	fences := []domain.Geofence{
		{ID: "a", Name: "A", Latitude: -6.1754, Longitude: 106.8272, RadiusMeters: 100},
		{ID: "b", Name: "B", Latitude: -6.1755, Longitude: 106.8273, RadiusMeters: 100},
	}
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(fences, pub)

	uc.Evaluate(context.Background(), monasLoc("B1234XYZ"))

	if n := len(pub.Events()); n != 2 {
		t.Fatalf("expected 2 events (2 fences), got %d", n)
	}
}

func TestGeofenceUsecase_NoFences_NoPublish(t *testing.T) {
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(nil, pub)

	uc.Evaluate(context.Background(), monasLoc("B1234XYZ"))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events with no fences, got %d", n)
	}
}

// ============================================================
// Publish failure rollback
// ============================================================

func TestGeofenceUsecase_PublishError_RollsBackState(t *testing.T) {
	pub := newMockPublisher()
	pub.setErr(errors.New("rabbit down"))

	uc := newTestGeofenceUsecase(defaultFences(), pub)
	loc := monasLoc("B1234XYZ")

	uc.Evaluate(context.Background(), loc)
	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events on error, got %d", n)
	}

	pub.setErr(nil)
	uc.Evaluate(context.Background(), loc)

	if n := len(pub.Events()); n != 1 {
		t.Fatalf("expected 1 event after recovery, got %d", n)
	}
}

// ============================================================
// State store errors
// ============================================================

func TestGeofenceUsecase_EnterStateError_NoPublish(t *testing.T) {
	pub := newMockPublisher()
	state := newMockStateStore()
	state.enterErr = errors.New("state store failure")

	uc := newTestGeofenceUsecaseWithState(defaultFences(), pub, state)
	uc.Evaluate(context.Background(), monasLoc("B1234XYZ"))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events when state enter fails, got %d", n)
	}
}

func TestGeofenceUsecase_ExitStateError_StillContinues(t *testing.T) {
	pub := newMockPublisher()
	state := newMockStateStore()
	state.exitErr = errors.New("state store failure")

	uc := newTestGeofenceUsecaseWithState(defaultFences(), pub, state)

	// Outside the fence — exit path will be triggered, error should be logged
	// but not panic and not publish anything.
	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.1900, 106.8400))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events outside, got %d", n)
	}
	if state.exitCalls == 0 {
		t.Fatal("expected ExitIfInside to be called")
	}
}

func TestGeofenceUsecase_PublishError_ResetsState(t *testing.T) {
	pub := newMockPublisher()
	pub.setErr(errors.New("rabbit down"))

	state := newMockStateStore()
	uc := newTestGeofenceUsecaseWithState(defaultFences(), pub, state)

	uc.Evaluate(context.Background(), monasLoc("B1234XYZ"))

	if state.resetCalls != 1 {
		t.Fatalf("expected 1 reset call, got %d", state.resetCalls)
	}
}

// ============================================================
// Boundary correctness (Haversine)
// ============================================================

func TestGeofenceUsecase_JustOutside_NoPublish(t *testing.T) {
	// ~57m north of Monas center (radius=50) → outside
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.1754+0.000512, 106.8272))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events just outside radius, got %d", n)
	}
}

func TestGeofenceUsecase_JustInside_Publish(t *testing.T) {
	// ~40m north → inside
	pub := newMockPublisher()
	uc := newTestGeofenceUsecase(defaultFences(), pub)

	uc.Evaluate(context.Background(), locAt("B1234XYZ", -6.1754+0.000359, 106.8272))

	if n := len(pub.Events()); n != 1 {
		t.Fatalf("expected 1 event just inside radius, got %d", n)
	}
}

// ============================================================
// Publisher unhealthy path
// ============================================================

func TestGeofenceUsecase_UnhealthyPublisher_NoEvent(t *testing.T) {
	pub := newMockPublisher()
	pub.Close() // sets healthy=false

	uc := newTestGeofenceUsecase(defaultFences(), pub)
	uc.Evaluate(context.Background(), monasLoc("B1234XYZ"))

	if n := len(pub.Events()); n != 0 {
		t.Fatalf("expected 0 events when publisher unhealthy, got %d", n)
	}
}
