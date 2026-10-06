package domain

import "context"

// Geofence describes a circular area of interest.
type Geofence struct {
	ID           string  `mapstructure:"id"           json:"id"`
	Name         string  `mapstructure:"name"         json:"name"`
	Latitude     float64 `mapstructure:"latitude"     json:"latitude"`
	Longitude    float64 `mapstructure:"longitude"    json:"longitude"`
	RadiusMeters float64 `mapstructure:"radius_meters" json:"radius_meters"`
}

// GeofenceStateStore tracks per-(vehicle,geofence) inside/outside state.
type GeofenceStateStore interface {
	// EnterIfOutside returns true ONLY if the vehicle was previously OUTSIDE.
	// On true, the store records the vehicle as INSIDE.
	EnterIfOutside(ctx context.Context, vehicleID, geofenceID string) (bool, error)

	// ExitIfInside marks the vehicle as OUTSIDE. Returns true if it was INSIDE.
	ExitIfInside(ctx context.Context, vehicleID, geofenceID string) (bool, error)

	// Reset removes any state (used for rollback when publish fails, so the
	// next tick retries the entry event).
	Reset(ctx context.Context, vehicleID, geofenceID string)
}
