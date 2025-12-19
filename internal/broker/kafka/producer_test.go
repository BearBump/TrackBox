package kafka

import (
	"context"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
)

type fakeWriter struct {
	last []kafka.Message
	err  error
}

func (w *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	w.last = append([]kafka.Message{}, msgs...)
	return w.err
}

func TestProducer_Publish(t *testing.T) {
	fw := &fakeWriter{}
	p := newProducerWithWriter(fw)

	require.NoError(t, p.Publish(context.Background(), "t", []byte("k"), []byte("v")))
	require.Len(t, fw.last, 1)
	require.Equal(t, "t", fw.last[0].Topic)
	require.Equal(t, []byte("k"), fw.last[0].Key)
	require.Equal(t, []byte("v"), fw.last[0].Value)
}

func TestNewProducer(t *testing.T) {
	p := NewProducer([]string{"localhost:0"})
	require.NotNil(t, p)
}


