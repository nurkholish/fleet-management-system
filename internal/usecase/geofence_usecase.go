package usecase

import (
	"context"

	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
	"fleet-management-system/pkg/geo"
)

type GeofenceUsecase struct {
	fences []domain.Geofence
	pub    domain.EventPublisher
	state  domain.GeofenceStateStore
	log    zerolog.Logger
}

func NewGeofenceUsecase(
	fences []domain.Geofence,
	pub domain.EventPublisher,
	state domain.GeofenceStateStore,
	log zerolog.Logger,
) *GeofenceUsecase {
	return &GeofenceUsecase{
		fences: fences,
		pub:    pub,
		state:  state,
		log:    log,
	}
}

func (g *GeofenceUsecase) Evaluate(ctx context.Context, loc domain.Location) {
	for _, f := range g.fences {
		dist := geo.HaversineMeters(loc.Latitude, loc.Longitude, f.Latitude, f.Longitude)
		inside := dist <= f.RadiusMeters

		if !inside {
			if _, err := g.state.ExitIfInside(ctx, loc.VehicleID, f.ID); err != nil {
				g.log.Warn().Err(err).Str("vehicle", loc.VehicleID).Str("geofence", f.ID).Msg("state exit failed")
			}
			continue
		}

		entered, err := g.state.EnterIfOutside(ctx, loc.VehicleID, f.ID)
		if err != nil {
			g.log.Warn().Err(err).Str("vehicle", loc.VehicleID).Str("geofence", f.ID).Msg("state enter failed")
			continue
		}
		if !entered {
			continue // already inside — no event
		}

		evt := domain.GeofenceEvent{
			VehicleID: loc.VehicleID,
			Event:     "geofence_entry",
			Location: domain.GeoPoint{
				Latitude:  loc.Latitude,
				Longitude: loc.Longitude,
			},
			Timestamp:  loc.Timestamp,
			GeofenceID: f.ID,
		}

		if err := g.pub.PublishGeofenceEntry(ctx, evt); err != nil {
			// Rollback so the next tick retries the entry.
			g.state.Reset(ctx, loc.VehicleID, f.ID)
			g.log.Warn().Err(err).
				Str("vehicle", loc.VehicleID).
				Str("geofence", f.ID).
				Msg("publish failed, state rolled back")
			continue
		}

		g.log.Info().
			Str("vehicle", loc.VehicleID).
			Str("geofence", f.ID).
			Float64("distance_m", dist).
			Msg("geofence_entry published")
	}
}
