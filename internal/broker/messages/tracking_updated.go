package messages

import (
	"encoding/json"
	"time"
)

type TrackingUpdated struct {
	TrackingID uint64 `json:"tracking_id"`
	CheckedAt  time.Time `json:"checked_at"`

	Status    string `json:"status,omitempty"`
	StatusRaw string `json:"status_raw,omitempty"`
	StatusAt  *time.Time `json:"status_at,omitempty"`

	NextCheckAt time.Time `json:"next_check_at"`

	Events []TrackingEvent `json:"events,omitempty"`

	Error *string `json:"error,omitempty"`
}

type TrackingEvent struct {
	Status    string `json:"status"`
	StatusRaw string `json:"status_raw"`
	EventTime time.Time `json:"event_time"`
	Location  *string `json:"location,omitempty"`
	Message   *string `json:"message,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}


