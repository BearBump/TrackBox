package pgtracking

import (
	"context"
	"testing"
	"time"

	"trackbox/internal/models"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPGTracking_RepoFlow(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "admin",
			"POSTGRES_PASSWORD": "admin",
			"POSTGRES_DB":       "trackbox_test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	pgC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	host, err := pgC.Host(ctx)
	require.NoError(t, err)
	port, err := pgC.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	dsn := "postgres://admin:admin@" + host + ":" + port.Port() + "/trackbox_test?sslmode=disable"
	st, err := New(dsn)
	require.NoError(t, err)
	t.Cleanup(st.Close)

	created, err := st.CreateOrGetTrackings(ctx, []models.TrackingCreateInput{
		{CarrierCode: "CDEK", TrackNumber: "A1"},
		{CarrierCode: "POST_RU", TrackNumber: "B2"},
	})
	require.NoError(t, err)
	require.Len(t, created, 2)
	require.NotZero(t, created[0].ID)

	// Делаем ровно один трек "due" и проверяем ClaimDueTrackings + lease
	_, err = st.db.Exec(ctx, `UPDATE trackings SET next_check_at = now() - interval '1 minute' WHERE id = $1`, created[0].ID)
	require.NoError(t, err)
	_, err = st.db.Exec(ctx, `UPDATE trackings SET next_check_at = now() + interval '1 hour' WHERE id = $1`, created[1].ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	lease := 10 * time.Second
	due, err := st.ClaimDueTrackings(ctx, now, 10, lease)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, created[0].ID, due[0].ID)
	require.WithinDuration(t, now.Add(lease), due[0].NextCheckAt, 2*time.Second)

	// апдейт статуса + событие
	evTime := time.Now().UTC()
	err = st.ApplyTrackingUpdate(ctx, TrackingUpdate{
		TrackingID:  created[0].ID,
		CheckedAt:   now,
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		StatusAt:    &now,
		NextCheckAt: now.Add(30 * time.Minute),
		Events: []*models.TrackingEvent{
			{Status: models.TrackingStatusInTransit, StatusRaw: "RAW", EventTime: evTime},
		},
	})
	require.NoError(t, err)

	evs, err := st.ListTrackingEvents(ctx, created[0].ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.WithinDuration(t, evTime, evs[0].EventTime, time.Second)

	// refresh
	require.NoError(t, st.RefreshTracking(ctx, created[0].ID))
}


