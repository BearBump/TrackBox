package pgtracking

import (
	"context"
	"time"

	"trackbox/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

const (
	defaultInitialStatus    = models.TrackingStatusUnknown
	defaultInitialStatusRaw = "UNKNOWN"
)

func (s *Storage) CreateOrGetTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error) {
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "begin tx")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids := make([]uint64, 0, len(items))
	for _, it := range items {
		var id uint64
		err := tx.QueryRow(ctx, `
INSERT INTO trackings (
  carrier_code, track_number, status, status_raw, next_check_at, created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$6)
ON CONFLICT (carrier_code, track_number)
DO UPDATE SET updated_at = trackings.updated_at
RETURNING id
`, it.CarrierCode, it.TrackNumber, defaultInitialStatus, defaultInitialStatusRaw, now, now).Scan(&id)
		if err != nil {
			return nil, errors.Wrap(err, "insert tracking")
		}
		ids = append(ids, id)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, errors.Wrap(err, "commit tx")
	}

	return s.GetTrackingsByIDs(ctx, ids)
}

func (s *Storage) GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error) {
	if len(ids) == 0 {
		return []*models.Tracking{}, nil
	}

	rows, err := s.db.Query(ctx, `
SELECT
  id, carrier_code, track_number,
  status, status_raw,
  status_at, last_checked_at, next_check_at,
  check_fail_count, last_error,
  created_at, updated_at
FROM trackings
WHERE id = ANY($1)
`, ids)
	if err != nil {
		return nil, errors.Wrap(err, "select trackings")
	}
	defer rows.Close()

	out := make([]*models.Tracking, 0, len(ids))
	for rows.Next() {
		var t models.Tracking
		var statusAt *time.Time
		var lastCheckedAt *time.Time
		var lastError *string
		if err := rows.Scan(
			&t.ID, &t.CarrierCode, &t.TrackNumber,
			&t.Status, &t.StatusRaw,
			&statusAt, &lastCheckedAt, &t.NextCheckAt,
			&t.CheckFailCount, &lastError,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan tracking")
		}
		t.StatusAt = statusAt
		t.LastCheckedAt = lastCheckedAt
		t.LastError = lastError
		out = append(out, &t)
	}

	if rows.Err() != nil {
		return nil, errors.Wrap(rows.Err(), "rows")
	}
	return out, nil
}

func (s *Storage) RefreshTracking(ctx context.Context, trackingID uint64) error {
	_, err := s.db.Exec(ctx, `UPDATE trackings SET next_check_at = now(), updated_at = now() WHERE id = $1`, trackingID)
	return errors.Wrap(err, "refresh tracking")
}

// ClaimDueTrackings выбирает пачку треков, готовых к проверке, и "бронирует" их,
// чтобы они не попадали в повторную выборку, пока воркер их обрабатывает.
// Использует SELECT ... FOR UPDATE SKIP LOCKED.
func (s *Storage) ClaimDueTrackings(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]*models.Tracking, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "begin tx")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
SELECT
  id, carrier_code, track_number,
  status, status_raw,
  status_at, last_checked_at, next_check_at,
  check_fail_count, last_error,
  created_at, updated_at
FROM trackings
WHERE next_check_at <= $1
  AND status <> $2
ORDER BY next_check_at ASC
LIMIT $3
FOR UPDATE SKIP LOCKED
`, now.UTC(), models.TrackingStatusDelivered, limit)
	if err != nil {
		return nil, errors.Wrap(err, "select due trackings")
	}
	defer rows.Close()

	var picked []*models.Tracking
	for rows.Next() {
		var t models.Tracking
		var statusAt *time.Time
		var lastCheckedAt *time.Time
		var lastError *string
		if err := rows.Scan(
			&t.ID, &t.CarrierCode, &t.TrackNumber,
			&t.Status, &t.StatusRaw,
			&statusAt, &lastCheckedAt, &t.NextCheckAt,
			&t.CheckFailCount, &lastError,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan due tracking")
		}
		t.StatusAt = statusAt
		t.LastCheckedAt = lastCheckedAt
		t.LastError = lastError
		picked = append(picked, &t)
	}
	if rows.Err() != nil {
		return nil, errors.Wrap(rows.Err(), "rows")
	}

	leaseUntil := now.UTC().Add(lease)
	for _, t := range picked {
		_, err := tx.Exec(ctx, `UPDATE trackings SET next_check_at = $2, updated_at = now() WHERE id = $1`, t.ID, leaseUntil)
		if err != nil {
			return nil, errors.Wrap(err, "lease tracking")
		}
		t.NextCheckAt = leaseUntil
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, errors.Wrap(err, "commit tx")
	}
	return picked, nil
}


