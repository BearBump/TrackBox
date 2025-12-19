package fake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeClient_GetTracking(t *testing.T) {
	c := New()
	res, err := c.GetTracking(context.Background(), "CDEK", "A1")
	require.NoError(t, err)
	require.NotEmpty(t, res.Status)
	require.NotNil(t, res.StatusAt)
	require.Len(t, res.Events, 1)
}


