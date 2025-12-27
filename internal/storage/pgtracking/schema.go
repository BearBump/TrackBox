package pgtracking

import (
	"context"

	"github.com/pkg/errors"
)

func (s *Storage) initSchema(ctx context.Context) error {
	stmts := []string{
		`
CREATE TABLE IF NOT EXISTS trackings (
  id BIGSERIAL PRIMARY KEY,
  carrier_code TEXT NOT NULL,
  track_number TEXT NOT NULL,
  status TEXT NOT NULL,
  status_raw TEXT NOT NULL,
  status_at TIMESTAMPTZ NULL,
  last_checked_at TIMESTAMPTZ NULL,
  next_check_at TIMESTAMPTZ NOT NULL,
  check_fail_count INT NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (carrier_code, track_number)
)`,
		`CREATE INDEX IF NOT EXISTS idx_trackings_next_check_at ON trackings(next_check_at)`,
		`
CREATE TABLE IF NOT EXISTS tracking_events (
  id BIGSERIAL PRIMARY KEY,
  tracking_id BIGINT NOT NULL REFERENCES trackings(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  status_raw TEXT NOT NULL,
  event_time TIMESTAMPTZ NOT NULL,
  location TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  payload JSONB NULL,
  created_at TIMESTAMPTZ NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_tracking_events_tracking_id_event_time ON tracking_events(tracking_id, event_time DESC)`,
		// Migrations for older schemas (when location/message were nullable).
		`UPDATE tracking_events SET location = '' WHERE location IS NULL`,
		`UPDATE tracking_events SET message = '' WHERE message IS NULL`,
		`ALTER TABLE tracking_events ALTER COLUMN location SET DEFAULT ''`,
		`ALTER TABLE tracking_events ALTER COLUMN message SET DEFAULT ''`,
		`ALTER TABLE tracking_events ALTER COLUMN location SET NOT NULL`,
		`ALTER TABLE tracking_events ALTER COLUMN message SET NOT NULL`,
		// Remove existing duplicates (so we can create unique index).
		`
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY tracking_id, status_raw, event_time, location, message
           ORDER BY id
         ) AS rn
  FROM tracking_events
)
DELETE FROM tracking_events
WHERE id IN (SELECT id FROM ranked WHERE rn > 1)
`,
		// Enforce de-duplication of events for a tracking.
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_tracking_events_dedup ON tracking_events(tracking_id, status_raw, event_time, location, message)`,
	}

	for _, q := range stmts {
		if _, err := s.db.Exec(ctx, q); err != nil {
			return errors.Wrap(err, "init schema")
		}
	}
	return nil
}


