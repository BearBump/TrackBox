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
  location TEXT NULL,
  message TEXT NULL,
  payload JSONB NULL,
  created_at TIMESTAMPTZ NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_tracking_events_tracking_id_event_time ON tracking_events(tracking_id, event_time DESC)`,
	}

	for _, q := range stmts {
		if _, err := s.db.Exec(ctx, q); err != nil {
			return errors.Wrap(err, "init schema")
		}
	}
	return nil
}


