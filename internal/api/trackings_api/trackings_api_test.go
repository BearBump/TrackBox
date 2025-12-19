package trackings_api

import (
	"context"
	"testing"
	"time"

	"github.com/BearBump/TrackBox/internal/models"
	pb_models "github.com/BearBump/TrackBox/internal/pb/models"
	"github.com/BearBump/TrackBox/internal/pb/trackings_api"
	"github.com/BearBump/TrackBox/internal/services/trackings"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/stretchr/testify/require"
)

type repo struct {
	created []*models.Tracking
	events  []*models.TrackingEvent
}

func (r *repo) CreateOrGetTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error) {
	return r.created, nil
}
func (r *repo) GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error) {
	return r.created, nil
}
func (r *repo) ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error) {
	return r.events, nil
}
func (r *repo) RefreshTracking(ctx context.Context, trackingID uint64) error { return nil }
func (r *repo) ApplyTrackingUpdate(ctx context.Context, upd pgtracking.TrackingUpdate) error { return nil }

func TestTrackingsAPI_Flow(t *testing.T) {
	now := time.Now().UTC()
	r := &repo{
		created: []*models.Tracking{{
			ID:          1,
			CarrierCode: "CDEK",
			TrackNumber: "A1",
			Status:      models.TrackingStatusUnknown,
			StatusRaw:   "UNKNOWN",
			NextCheckAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}},
		events: []*models.TrackingEvent{{
			ID:         10,
			TrackingID: 1,
			Status:     "IN_TRANSIT",
			StatusRaw:  "RAW",
			EventTime:  now,
			CreatedAt:  now,
		}},
	}

	svc := trackings.New(r, nil, 0)
	api := New(svc)

	created, err := api.CreateTrackings(context.Background(), &trackings_api.CreateTrackingsRequest{
		Items: []*pb_models.TrackingCreateInput{{CarrierCode: "CDEK", TrackNumber: "A1"}},
	})
	require.NoError(t, err)
	require.Len(t, created.Trackings, 1)
	require.Equal(t, uint64(1), created.Trackings[0].Id)

	byIDs, err := api.GetTrackingsByIds(context.Background(), &trackings_api.GetTrackingsByIdsRequest{Ids: []uint64{1}})
	require.NoError(t, err)
	require.Len(t, byIDs.Trackings, 1)

	evs, err := api.ListTrackingEvents(context.Background(), &trackings_api.ListTrackingEventsRequest{
		TrackingId: 1,
		Limit:      10,
		Offset:     0,
	})
	require.NoError(t, err)
	require.Len(t, evs.Events, 1)

	_, err = api.RefreshTracking(context.Background(), &trackings_api.RefreshTrackingRequest{TrackingId: 1})
	require.NoError(t, err)
}

func TestDerefString(t *testing.T) {
	require.Equal(t, "", derefString(nil))
	s := "x"
	require.Equal(t, "x", derefString(&s))
}


