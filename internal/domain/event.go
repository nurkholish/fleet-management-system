package domain

import "context"

type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GeofenceEvent is the wire format published to RabbitMQ.
// GeofenceID is intentionally excluded from JSON per spec.
type GeofenceEvent struct {
	VehicleID  string   `json:"vehicle_id"`
	Event      string   `json:"event"`
	Location   GeoPoint `json:"location"`
	Timestamp  int64    `json:"timestamp"`
	GeofenceID string   `json:"-"`
}

// EventPublisher abstracts the message broker (hexagonal port).
type EventPublisher interface {
	PublishGeofenceEntry(ctx context.Context, evt GeofenceEvent) error
	IsHealthy() bool
	Close() error
}
