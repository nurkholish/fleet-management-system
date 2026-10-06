package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"fleet-management-system/internal/domain"
)

func pgDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TJ_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("TJ_POSTGRES_TEST_DSN not set, skipping integration test")
	}
	return dsn
}

func setupRepo(t *testing.T) (*LocationRepo, func()) {
	t.Helper()
	dsn := pgDSN(t)
	ctx := context.Background()

	pool, err := NewPool(ctx, dsn, 1, 5, time.Minute)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS vehicle_locations (
			id BIGSERIAL PRIMARY KEY,
			vehicle_id VARCHAR(32) NOT NULL,
			latitude DOUBLE PRECISION NOT NULL,
			longitude DOUBLE PRECISION NOT NULL,
			timestamp BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_ts_unique
			ON vehicle_locations (vehicle_id, timestamp);
		CREATE INDEX IF NOT EXISTS idx_vehicle_ts_desc
			ON vehicle_locations (vehicle_id, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_vehicle_ts_range
			ON vehicle_locations (vehicle_id, timestamp);
	`)
	if err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}

	if _, err := pool.Exec(ctx, `TRUNCATE vehicle_locations RESTART IDENTITY`); err != nil {
		pool.Close()
		t.Fatalf("truncate: %v", err)
	}

	repo := NewLocationRepo(pool)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `TRUNCATE vehicle_locations RESTART IDENTITY`)
		pool.Close()
	}
	return repo, cleanup
}

func TestLocationRepo_SaveAndGetLatest(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()

	loc := domain.Location{
		VehicleID: "B1234XYZ",
		Latitude:  -6.2088,
		Longitude: 106.8456,
		Timestamp: now,
	}
	if err := repo.Save(ctx, loc); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.GetLatest(ctx, "B1234XYZ")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got.VehicleID != loc.VehicleID {
		t.Errorf("vehicle mismatch: %s", got.VehicleID)
	}
	if got.Latitude != loc.Latitude {
		t.Errorf("lat mismatch: %f", got.Latitude)
	}
	if got.Timestamp != loc.Timestamp {
		t.Errorf("ts mismatch: %d vs %d", got.Timestamp, loc.Timestamp)
	}
}

func TestLocationRepo_Save_Idempotent(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()

	loc := domain.Location{
		VehicleID: "B1234XYZ",
		Latitude:  -6.2088,
		Longitude: 106.8456,
		Timestamp: now,
	}
	if err := repo.Save(ctx, loc); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := repo.Save(ctx, loc); err != nil {
		t.Fatalf("duplicate save: %v", err)
	}

	var count int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_locations WHERE vehicle_id = $1`, loc.VehicleID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after duplicate insert, got %d", count)
	}
}

func TestLocationRepo_GetLatest_NotFound(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	_, err := repo.GetLatest(context.Background(), "UNKNOWN")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocationRepo_GetHistory(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Now().Unix()

	for i := 0; i < 5; i++ {
		if err := repo.Save(ctx, domain.Location{
			VehicleID: "B1234XYZ",
			Latitude:  -6.2,
			Longitude: 106.8,
			Timestamp: base + int64(i),
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	hist, nextCursor, hasMore, err := repo.GetHistory(ctx, "B1234XYZ", base-1, base+10, 0, 100)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(hist) != 5 {
		t.Fatalf("expected 5, got %d", len(hist))
	}
	if hasMore {
		t.Errorf("expected hasMore=false, got true")
	}
	if nextCursor != 0 {
		t.Errorf("expected nextCursor=0, got %d", nextCursor)
	}
	for i := 1; i < len(hist); i++ {
		if hist[i].Timestamp < hist[i-1].Timestamp {
			t.Errorf("history not sorted: %d < %d", hist[i].Timestamp, hist[i-1].Timestamp)
		}
	}
}

func TestLocationRepo_GetHistory_Pagination(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Now().Unix()

	const total = 5
	for i := 0; i < total; i++ {
		if err := repo.Save(ctx, domain.Location{
			VehicleID: "B1234XYZ",
			Latitude:  -6.2,
			Longitude: 106.8,
			Timestamp: base + int64(i),
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	page1, cursor1, hasMore1, err := repo.GetHistory(ctx, "B1234XYZ", base-1, base+10, 0, 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: expected 2 rows, got %d", len(page1))
	}
	if !hasMore1 {
		t.Fatalf("page1: expected hasMore=true")
	}
	if cursor1 == 0 {
		t.Fatalf("page1: expected non-zero cursor")
	}

	page2, cursor2, hasMore2, err := repo.GetHistory(ctx, "B1234XYZ", base-1, base+10, cursor1, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2: expected 2 rows, got %d", len(page2))
	}
	if !hasMore2 {
		t.Fatalf("page2: expected hasMore=true")
	}
	if cursor2 == 0 {
		t.Fatalf("page2: expected non-zero cursor")
	}

	page3, cursor3, hasMore3, err := repo.GetHistory(ctx, "B1234XYZ", base-1, base+10, cursor2, 2)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3: expected 1 row, got %d", len(page3))
	}
	if hasMore3 {
		t.Errorf("page3: expected hasMore=false, got true")
	}
	if cursor3 != 0 {
		t.Errorf("page3: expected no cursor (last page), got %d", cursor3)
	}

	all := append(append(append([]domain.Location{}, page1...), page2...), page3...)
	if len(all) != total {
		t.Fatalf("total: expected %d, got %d", total, len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Timestamp <= all[i-1].Timestamp {
			t.Errorf("pagination overlap/unordered at %d: %d <= %d",
				i, all[i].Timestamp, all[i-1].Timestamp)
		}
	}
}

func TestLocationRepo_GetHistory_EmptyRange(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	hist, cursor, hasMore, err := repo.GetHistory(context.Background(), "B1234XYZ", 1, 100, 0, 100)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("expected 0, got %d", len(hist))
	}
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if hasMore {
		t.Errorf("expected hasMore=false, got true")
	}
}

func TestLocationRepo_Ping(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	if err := repo.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
