package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
)

type fakeReader struct {
	msgs []kafka.Message
	err  error
	i    int
}

func (r *fakeReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if r.i < len(r.msgs) {
		m := r.msgs[r.i]
		r.i++
		return m, nil
	}
	if r.err != nil {
		return kafka.Message{}, r.err
	}
	return kafka.Message{}, errors.New("eof")
}

func (r *fakeReader) Close() error { return nil }

func TestConsumer_Consume_CallsHandler(t *testing.T) {
	fr := &fakeReader{
		msgs: []kafka.Message{{Key: []byte("k"), Value: []byte("v")}},
		err:  errors.New("stop"),
	}
	c := newConsumerWithReader(fr)

	var gotK, gotV []byte
	err := c.Consume(context.Background(), func(k, v []byte) error {
		gotK, gotV = k, v
		return nil
	})
	require.Error(t, err)
	require.Equal(t, []byte("k"), gotK)
	require.Equal(t, []byte("v"), gotV)
}

func TestConsumer_Consume_HandlerErrorStops(t *testing.T) {
	fr := &fakeReader{msgs: []kafka.Message{{Key: []byte("k"), Value: []byte("v")}}}
	c := newConsumerWithReader(fr)

	want := errors.New("handler failed")
	err := c.Consume(context.Background(), func(k, v []byte) error { return want })
	require.ErrorIs(t, err, want)
}

func TestNewConsumer_Close(t *testing.T) {
	c := NewConsumer([]string{"localhost:0"}, "t", "g")
	require.NotNil(t, c)
	require.NoError(t, c.Close())
}


