package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fleet-management-system/internal/domain"
)

// ============================================================
// Mock repository
// ============================================================

type mockLocRepo struct {
	mu sync.Mutex

	saved     []domain.Location
	latest    domain.Location
	latestErr error
	hist      []domain.Location
	nextCur   int64
	hasMore   bool
	histErr   error
	saveErr   error

	// capture last GetHistory args for assertion
	lastVehicleID string
	lastStart     int64
	lastEnd       int64
	lastCursor    int64
	lastLimit     int
}

func (m *mockLocRepo) Save(_ context.Context, loc domain.Location) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, loc)
	return nil
}

func (m *mockLocRepo) GetLatest(_ context.Context, vehicleID string) (domain.Location, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastVehicleID = vehicleID
	return m.latest, m.latestErr
}

func (m *mockLocRepo) GetHistory(
	_ context.Context,
	vehicleID string,
	start, end, cursor int64,
	limit int,
) ([]domain.Location, int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastVehicleID = vehicleID
	m.lastStart = start
	m.lastEnd = end
	m.lastCursor = cursor
	m.lastLimit = limit
	return m.hist, m.nextCur, m.hasMore, m.histErr
}

// ============================================================
// Helpers
// ============================================================

func validLoc(vehicleID string) domain.Location {
	return domain.Location{
		VehicleID: vehicleID,
		Latitude:  -6.2,
		Longitude: 106.8,
		Timestamp: time.Now().Unix(),
	}
}

// ============================================================
// Ingest
// ============================================================

func TestLocationUsecase_Ingest_Valid(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	if err := uc.Ingest(context.Background(), validLoc("B1234XYZ")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 save, got %d", len(repo.saved))
	}
	if repo.saved[0].VehicleID != "B1234XYZ" {
		t.Errorf("vehicle=%q, want B1234XYZ", repo.saved[0].VehicleID)
	}
}

func TestLocationUsecase_Ingest_InvalidLocation(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	// invalid plate
	bad := domain.Location{
		VehicleID: "invalid",
		Latitude:  -6.2,
		Longitude: 106.8,
		Timestamp: time.Now().Unix(),
	}
	err := uc.Ingest(context.Background(), bad)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
	if len(repo.saved) != 0 {
		t.Fatal("repo should not be called when validation fails")
	}
}

func TestLocationUsecase_Ingest_RepoError(t *testing.T) {
	repo := &mockLocRepo{saveErr: errors.New("db down")}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	err := uc.Ingest(context.Background(), validLoc("B1234XYZ"))
	if err == nil {
		t.Fatal("expected error")
	}
}

// ============================================================
// GetLatest
// ============================================================

func TestLocationUsecase_GetLatest_EmptyID(t *testing.T) {
	uc := NewLocationUsecase(&mockLocRepo{}, 604800, 1000, 10000)
	_, err := uc.GetLatest(context.Background(), "")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestLocationUsecase_GetLatest_Success(t *testing.T) {
	expected := validLoc("B1234XYZ")
	repo := &mockLocRepo{latest: expected}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	got, err := uc.GetLatest(context.Background(), "B1234XYZ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.VehicleID != expected.VehicleID {
		t.Errorf("vehicle=%q, want %q", got.VehicleID, expected.VehicleID)
	}
	if repo.lastVehicleID != "B1234XYZ" {
		t.Errorf("repo received vehicle=%q, want B1234XYZ", repo.lastVehicleID)
	}
}

func TestLocationUsecase_GetLatest_NotFound(t *testing.T) {
	repo := &mockLocRepo{latestErr: domain.ErrNotFound}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	_, err := uc.GetLatest(context.Background(), "B1234XYZ")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// GetHistory
// ============================================================

func TestLocationUsecase_GetHistory_InvalidVehicleID(t *testing.T) {
	uc := NewLocationUsecase(&mockLocRepo{}, 604800, 1000, 10000)
	_, err := uc.GetHistory(context.Background(), "", 100, 200, 0, 100)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestLocationUsecase_GetHistory_InvalidRange(t *testing.T) {
	uc := NewLocationUsecase(&mockLocRepo{}, 604800, 1000, 10000)
	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 50, 0, 100)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestLocationUsecase_GetHistory_EqualStartEnd(t *testing.T) {
	uc := NewLocationUsecase(&mockLocRepo{}, 604800, 1000, 10000)
	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 100, 0, 100)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for start==end, got %v", err)
	}
}

func TestLocationUsecase_GetHistory_ExceedsMaxRange(t *testing.T) {
	uc := NewLocationUsecase(&mockLocRepo{}, 60, 1000, 10000)
	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 100)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestLocationUsecase_GetHistory_Success(t *testing.T) {
	repo := &mockLocRepo{
		hist: []domain.Location{
			validLoc("B1234XYZ"),
			validLoc("B1234XYZ"),
		},
		nextCur: 999,
		hasMore: false,
	}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	page, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.VehicleID != "B1234XYZ" {
		t.Errorf("vehicle=%q, want B1234XYZ", page.VehicleID)
	}
	if page.Count != 2 {
		t.Errorf("count=%d, want 2", page.Count)
	}
	if page.NextCursor != 999 {
		t.Errorf("nextCursor=%d, want 999", page.NextCursor)
	}
	if page.HasMore {
		t.Errorf("hasMore=true, want false")
	}
	if len(page.Data) != 2 {
		t.Errorf("len(data)=%d, want 2", len(page.Data))
	}
}

func TestLocationUsecase_GetHistory_LimitClampedToMax(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	// limit 999999 > maxLimit 10000 → should be clamped
	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 999999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != 10000 {
		t.Errorf("limit=%d, want 10000 (clamped)", repo.lastLimit)
	}
}

func TestLocationUsecase_GetHistory_LimitDefaultedWhenZero(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != 1000 {
		t.Errorf("limit=%d, want 1000 (default)", repo.lastLimit)
	}
}

func TestLocationUsecase_GetHistory_LimitDefaultedWhenNegative(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != 1000 {
		t.Errorf("limit=%d, want 1000 (default)", repo.lastLimit)
	}
}

func TestLocationUsecase_GetHistory_CursorForwarded(t *testing.T) {
	repo := &mockLocRepo{}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 555, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastCursor != 555 {
		t.Errorf("cursor=%d, want 555", repo.lastCursor)
	}
}

func TestLocationUsecase_GetHistory_RepoError(t *testing.T) {
	repo := &mockLocRepo{histErr: errors.New("db down")}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	_, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLocationUsecase_GetHistory_EmptyResult(t *testing.T) {
	repo := &mockLocRepo{hist: nil}
	uc := NewLocationUsecase(repo, 604800, 1000, 10000)

	page, err := uc.GetHistory(context.Background(), "B1234XYZ", 100, 200, 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Count != 0 {
		t.Errorf("count=%d, want 0", page.Count)
	}
	if page.Data == nil {
		t.Errorf("data should be non-nil empty slice, got nil")
	}
}
