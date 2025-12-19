package models

import "time"

// Нормализованные статусы (можно расширять).
const (
	TrackingStatusUnknown   = "UNKNOWN"
	TrackingStatusInTransit = "IN_TRANSIT"
	TrackingStatusDelivered = "DELIVERED"
)

type Tracking struct {
	ID           uint64
	CarrierCode  string
	TrackNumber  string
	Status       string
	StatusRaw    string
	StatusAt     *time.Time
	LastCheckedAt *time.Time
	NextCheckAt  time.Time
	CheckFailCount int32
	LastError    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type TrackingEvent struct {
	ID         uint64
	TrackingID uint64
	Status     string
	StatusRaw  string
	EventTime  time.Time
	Location   *string
	Message    *string
	PayloadJSON *string
	CreatedAt  time.Time
}

type TrackingCreateInput struct {
	CarrierCode string
	TrackNumber string
}


