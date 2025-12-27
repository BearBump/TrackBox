package pgtracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BearBump/TrackBox/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

type TrackingUpdate struct {
	TrackingID uint64

	CheckedAt time.Time

	Status    string
	StatusRaw string
	StatusAt  *time.Time

	NextCheckAt time.Time

	Events []*models.TrackingEvent

	Error *string
}

func (s *Storage) ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(ctx, `
SELECT
  id, tracking_id, status, status_raw,
  event_time, location, message, payload, created_at
FROM tracking_events
WHERE tracking_id = $1
ORDER BY event_time DESC
LIMIT $2 OFFSET $3
`, trackingID, limit, offset)
	if err != nil {
		return nil, errors.Wrap(err, "select events")
	}
	defer rows.Close()

	var out []*models.TrackingEvent
	for rows.Next() {
		var e models.TrackingEvent
		var location *string
		var message *string
		var payload any
		if err := rows.Scan(
			&e.ID, &e.TrackingID, &e.Status, &e.StatusRaw,
			&e.EventTime, &location, &message, &payload, &e.CreatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan event")
		}

		e.Location = location
		e.Message = message

		if payload != nil {
			b, _ := json.Marshal(payload)
			s := string(b)
			e.PayloadJSON = &s
		}

		out = append(out, &e)
	}
	if rows.Err() != nil {
		return nil, errors.Wrap(rows.Err(), "rows")
	}
	return out, nil
}

func (s *Storage) ApplyTrackingUpdate(ctx context.Context, upd TrackingUpdate) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "begin tx")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if upd.Error != nil && *upd.Error != "" {
		_, err := tx.Exec(ctx, `
UPDATE trackings
SET
  last_checked_at = $2,
  check_fail_count = check_fail_count + 1,
  last_error = $3,
  next_check_at = $4,
  updated_at = now()
WHERE id = $1
`, upd.TrackingID, upd.CheckedAt.UTC(), *upd.Error, upd.NextCheckAt.UTC())
		if err != nil {
			return errors.Wrap(err, "update tracking (error)")
		}
	} else {
		_, err := tx.Exec(ctx, `
UPDATE trackings
SET
  status = $3,
  status_raw = $4,
  status_at = $5,
  last_checked_at = $2,
  check_fail_count = 0,
  last_error = NULL,
  next_check_at = $6,
  updated_at = now()
WHERE id = $1
`, upd.TrackingID, upd.CheckedAt.UTC(), upd.Status, upd.StatusRaw, upd.StatusAt, upd.NextCheckAt.UTC())
		if err != nil {
			return errors.Wrap(err, "update tracking (ok)")
		}

		for _, e := range upd.Events {
			var payload any
			if e.PayloadJSON != nil && *e.PayloadJSON != "" {
				var m any
				if json.Unmarshal([]byte(*e.PayloadJSON), &m) == nil {
					payload = m
				}
			}

			loc := ""
			if e.Location != nil {
				loc = *e.Location
			}
			msgText := ""
			if e.Message != nil {
				msgText = *e.Message
			}

			_, err := tx.Exec(ctx, `
INSERT INTO tracking_events (
  tracking_id, status, status_raw, event_time, location, message, payload, created_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7, now())
ON CONFLICT (tracking_id, status_raw, event_time, location, message) DO NOTHING
`, upd.TrackingID, e.Status, e.StatusRaw, e.EventTime.UTC(), loc, msgText, payload)
			if err != nil {
				return errors.Wrap(err, "insert tracking event")
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit tx")
	}
	return nil
}


