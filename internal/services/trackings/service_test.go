package trackings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	createIn []models.TrackingCreateInput
	createOut []*models.Tracking
	createErr error

	refreshID uint64
	refreshErr error

	getIn []uint64
	getOut []*models.Tracking
	getErr error

	applyUpd pgtracking.TrackingUpdate
	applyErr error
}

func (f *fakeRepo) CreateOrGetTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error) {
	f.createIn = items
	return f.createOut, f.createErr
}
func (f *fakeRepo) GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error) {
	f.getIn = ids
	return f.getOut, f.getErr
}
func (f *fakeRepo) ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error) {
	return nil, nil
}
func (f *fakeRepo) RefreshTracking(ctx context.Context, trackingID uint64) error {
	f.refreshID = trackingID
	return f.refreshErr
}
func (f *fakeRepo) ApplyTrackingUpdate(ctx context.Context, upd pgtracking.TrackingUpdate) error {
	f.applyUpd = upd
	return f.applyErr
}

type fakeCache struct {
	m map[string][]byte
}

func (c *fakeCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, ok := c.m[key]
	return b, ok, nil
}
func (c *fakeCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.m[key] = value
	return nil
}

func TestService_CreateTrackings_validate(t *testing.T) {
	s := New(&fakeRepo{}, nil, 0)
	_, err := s.CreateTrackings(context.Background(), nil)
	require.Error(t, err)

	_, err = s.CreateTrackings(context.Background(), []models.TrackingCreateInput{{CarrierCode: "", TrackNumber: "X"}})
	require.Error(t, err)

	_, err = s.CreateTrackings(context.Background(), []models.TrackingCreateInput{{CarrierCode: "C", TrackNumber: ""}})
	require.Error(t, err)
}

func TestService_CreateTrackings_dedup(t *testing.T) {
	r := &fakeRepo{createOut: []*models.Tracking{{ID: 1}}}
	s := New(r, nil, 0)

	_, err := s.CreateTrackings(context.Background(), []models.TrackingCreateInput{
		{CarrierCode: "C", TrackNumber: "A"},
		{CarrierCode: "C", TrackNumber: "A"},
		{CarrierCode: "C", TrackNumber: "B"},
	})
	require.NoError(t, err)
	require.Len(t, r.createIn, 2)
}

func TestService_RefreshTracking_validate(t *testing.T) {
	r := &fakeRepo{}
	s := New(r, nil, 0)
	require.Error(t, s.RefreshTracking(context.Background(), 0))

	require.NoError(t, s.RefreshTracking(context.Background(), 10))
	require.Equal(t, uint64(10), r.refreshID)
}

func TestService_GetTrackingsByIDs_cacheHit(t *testing.T) {
	r := &fakeRepo{}
	c := &fakeCache{m: map[string][]byte{}}
	s := New(r, c, 10*time.Minute)

	want := &models.Tracking{ID: 7, CarrierCode: "C", TrackNumber: "N", Status: "UNKNOWN"}
	b, _ := json.Marshal(want)
	c.m["tracking:7:current"] = b

	out, err := s.GetTrackingsByIDs(context.Background(), []uint64{7})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, uint64(7), out[0].ID)
	require.Nil(t, r.getIn) // БД не трогали
}

func TestService_ApplyKafkaUpdate_buildsUpdate(t *testing.T) {
	r := &fakeRepo{getOut: []*models.Tracking{{ID: 1}}}
	s := New(r, nil, 0)
	now := time.Now().UTC()

	msg := messages.TrackingUpdated{
		TrackingID: 1,
		CheckedAt:  now,
		Status:     "IN_TRANSIT",
		StatusRaw:  "RAW",
		NextCheckAt: now.Add(10 * time.Minute),
		Events: []messages.TrackingEvent{
			{Status: "IN_TRANSIT", StatusRaw: "RAW", EventTime: now},
		},
	}
	require.NoError(t, s.ApplyKafkaUpdate(context.Background(), msg))
	require.Equal(t, uint64(1), r.applyUpd.TrackingID)
	require.Equal(t, "IN_TRANSIT", r.applyUpd.Status)
	require.Len(t, r.applyUpd.Events, 1)
}


