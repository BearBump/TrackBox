package fake

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/models"
)

// FakeClient — временная заглушка "перевозчика" (пока python emulator не готов).
// Делаем детерминированный статус по (carrier, track_number): часть треков станет DELIVERED.
type FakeClient struct{}

func New() *FakeClient { return &FakeClient{} }

func (f *FakeClient) GetTracking(ctx context.Context, carrierCode, trackNumber string) (carrier.TrackingResult, error) {
	now := time.Now().UTC()

	h := fnv.New32a()
	_, _ = h.Write([]byte(carrierCode))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(trackNumber))
	v := h.Sum32()

	// 20% треков считаем доставленными
	status := models.TrackingStatusInTransit
	if v%5 == 0 {
		status = models.TrackingStatusDelivered
	}

	raw := status
	ev := &models.TrackingEvent{
		Status:    status,
		StatusRaw: raw,
		EventTime: now,
		Message:   ptr("fake carrier update"),
	}

	return carrier.TrackingResult{
		Status:    status,
		StatusRaw: raw,
		StatusAt:  &now,
		Events:    []*models.TrackingEvent{ev},
	}, nil
}

func ptr(s string) *string { return &s }


