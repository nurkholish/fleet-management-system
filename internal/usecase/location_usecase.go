package usecase

import (
	"context"
	"fmt"

	"fleet-management-system/internal/domain"
)

type LocationUsecase struct {
	repo            domain.LocationRepository
	maxRangeSeconds int64
	defaultLimit    int
	maxLimit        int
}

func NewLocationUsecase(repo domain.LocationRepository, maxRange int64, defLimit, maxLimit int) *LocationUsecase {
	return &LocationUsecase{
		repo:            repo,
		maxRangeSeconds: maxRange,
		defaultLimit:    defLimit,
		maxLimit:        maxLimit,
	}
}

func (u *LocationUsecase) Ingest(ctx context.Context, loc domain.Location) error {
	if err := loc.Validate(); err != nil {
		return fmt.Errorf("validate location: %w", err)
	}
	return u.repo.Save(ctx, loc)
}

func (u *LocationUsecase) GetLatest(ctx context.Context, vehicleID string) (domain.Location, error) {
	if vehicleID == "" {
		return domain.Location{}, domain.ErrValidation
	}
	return u.repo.GetLatest(ctx, vehicleID)
}

type HistoryPage struct {
	VehicleID  string            `json:"vehicle_id"`
	Count      int               `json:"count"`
	HasMore    bool              `json:"has_more"`
	NextCursor int64             `json:"next_cursor,omitempty"`
	Data       []domain.Location `json:"data"`
}

func (u *LocationUsecase) GetHistory(
	ctx context.Context,
	vehicleID string,
	start, end, cursor int64,
	limit int,
) (HistoryPage, error) {
	if vehicleID == "" || end <= start {
		return HistoryPage{}, fmt.Errorf("%w: invalid range", domain.ErrValidation)
	}
	if end-start > u.maxRangeSeconds {
		return HistoryPage{}, fmt.Errorf(
			"%w: range exceeds %d seconds", domain.ErrValidation, u.maxRangeSeconds)
	}
	if limit <= 0 {
		limit = u.defaultLimit
	}
	if limit > u.maxLimit {
		limit = u.maxLimit
	}

	locs, next, hasMore, err := u.repo.GetHistory(ctx, vehicleID, start, end, cursor, limit)
	if err != nil {
		return HistoryPage{}, err
	}

	if locs == nil {
		locs = []domain.Location{}
	}

	return HistoryPage{
		VehicleID:  vehicleID,
		Count:      len(locs),
		HasMore:    hasMore,
		NextCursor: next,
		Data:       locs,
	}, nil
}
