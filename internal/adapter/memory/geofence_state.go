package memory

import (
	"context"
	"sync"

	"fleet-management-system/internal/domain"
)

type GeofenceState struct {
	mu     sync.Mutex
	inside map[string]bool // key: vehicleID|geofenceID
}

func NewGeofenceState() *GeofenceState {
	return &GeofenceState{inside: make(map[string]bool)}
}

var _ domain.GeofenceStateStore = (*GeofenceState)(nil)

func key(vehicleID, geofenceID string) string {
	return vehicleID + "\x00" + geofenceID
}

func (s *GeofenceState) EnterIfOutside(_ context.Context, vehicleID, geofenceID string) (bool, error) {
	k := key(vehicleID, geofenceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inside[k] {
		return false, nil
	}
	s.inside[k] = true
	return true, nil
}

func (s *GeofenceState) ExitIfInside(_ context.Context, vehicleID, geofenceID string) (bool, error) {
	k := key(vehicleID, geofenceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.inside[k] {
		return false, nil
	}
	delete(s.inside, k)
	return true, nil
}

func (s *GeofenceState) Reset(_ context.Context, vehicleID, geofenceID string) {
	k := key(vehicleID, geofenceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inside, k)
}
