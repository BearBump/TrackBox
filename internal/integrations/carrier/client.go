package carrier

import (
	"context"
	"time"

	"trackbox/internal/models"
)

type TrackingResult struct {
	Status    string
	StatusRaw string
	StatusAt  *time.Time
	Events    []*models.TrackingEvent
}

type Client interface {
	GetTracking(ctx context.Context, carrierCode, trackNumber string) (TrackingResult, error)
}


