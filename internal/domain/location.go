package domain

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	ErrNotFound   = errors.New("location not found")
	ErrValidation = errors.New("validation error")
)

// Indonesian plate: B1234XYZ, D5678AB, B 1234 XYZ (normalized without spaces).
var vehicleIDPattern = regexp.MustCompile(`^[A-Z]{1,2}\d{1,4}[A-Z]{0,3}$`)

const (
	maxClockSkewSeconds = 300        // 5 minutes forward tolerance
	maxAgeSeconds       = 30 * 86400 // 30 days backward tolerance
)

type Location struct {
	VehicleID string  `json:"vehicle_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp int64   `json:"timestamp"`
}

// Validate ensures the location is domain-valid.
func (l Location) Validate() error {
	if !vehicleIDPattern.MatchString(l.VehicleID) {
		return ErrValidation
	}
	if l.Latitude < -90 || l.Latitude > 90 {
		return ErrValidation
	}
	if l.Longitude < -180 || l.Longitude > 180 {
		return ErrValidation
	}
	if l.Timestamp <= 0 {
		return ErrValidation
	}
	now := time.Now().Unix()
	if l.Timestamp > now+maxClockSkewSeconds {
		return ErrValidation
	}
	if l.Timestamp < now-maxAgeSeconds {
		return ErrValidation
	}
	return nil
}

type LocationRepository interface {
	Save(ctx context.Context, loc Location) error
	GetLatest(ctx context.Context, vehicleID string) (Location, error)
	// GetHistory returns rows, next cursor, and whether more pages exist.
	GetHistory(ctx context.Context, vehicleID string, start, end, cursor int64, limit int) ([]Location, int64, bool, error)
}
