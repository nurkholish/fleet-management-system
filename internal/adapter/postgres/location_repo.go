package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleet-management-system/internal/domain"
)

type LocationRepo struct {
	pool *pgxpool.Pool
}

func NewLocationRepo(pool *pgxpool.Pool) *LocationRepo {
	return &LocationRepo{pool: pool}
}

// NewPool constructs a pgxpool with explicit min/max conns.
func NewPool(
	ctx context.Context,
	dsn string,
	minConns, maxConns int32,
	maxLifetime time.Duration,
) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg dsn: %w", err)
	}
	cfg.MinConns = minConns
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = maxLifetime
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}
	return pool, nil
}

func (r *LocationRepo) Save(ctx context.Context, loc domain.Location) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO vehicle_locations (vehicle_id, latitude, longitude, timestamp)
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (vehicle_id, timestamp) DO NOTHING`,
		loc.VehicleID, loc.Latitude, loc.Longitude, loc.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert location: %w", err)
	}
	return nil
}

func (r *LocationRepo) GetLatest(ctx context.Context, vehicleID string) (domain.Location, error) {
	var loc domain.Location
	err := r.pool.QueryRow(ctx,
		`SELECT vehicle_id, latitude, longitude, timestamp
         FROM vehicle_locations
         WHERE vehicle_id = $1
         ORDER BY timestamp DESC
         LIMIT 1`,
		vehicleID,
	).Scan(&loc.VehicleID, &loc.Latitude, &loc.Longitude, &loc.Timestamp)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Location{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Location{}, fmt.Errorf("query latest: %w", err)
	}
	return loc, nil
}

// GetHistory returns rows ordered ascending, plus a cursor for the next page.
// Pass cursor=0 for the first page. hasMore reports whether another page exists.
func (r *LocationRepo) GetHistory(
	ctx context.Context,
	vehicleID string,
	start, end, cursor int64,
	limit int,
) ([]domain.Location, int64, bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT vehicle_id, latitude, longitude, timestamp
         FROM vehicle_locations
         WHERE vehicle_id = $1
           AND timestamp BETWEEN $2 AND $3
           AND ($4 = 0 OR timestamp > $4)
         ORDER BY timestamp ASC
         LIMIT $5`,
		vehicleID, start, end, cursor, limit+1, // fetch one extra to detect next page
	)
	if err != nil {
		return nil, 0, false, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Location, 0, limit)
	for rows.Next() {
		var l domain.Location
		if err := rows.Scan(&l.VehicleID, &l.Latitude, &l.Longitude, &l.Timestamp); err != nil {
			return nil, 0, false, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	var (
		nextCursor int64
		hasMore    bool
	)
	if len(out) > limit {
		hasMore = true
		nextCursor = out[limit-1].Timestamp + 1 // cursor is exclusive
		out = out[:limit]
	}
	return out, nextCursor, hasMore, nil
}

func (r *LocationRepo) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
