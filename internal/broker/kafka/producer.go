package kafka

import (
	"context"

	"github.com/pkg/errors"
	"github.com/segmentio/kafka-go"
)

type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type Producer struct {
	w messageWriter
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
		w: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Balancer: &kafka.LeastBytes{},
		},
	}
}

func newProducerWithWriter(w messageWriter) *Producer {
	return &Producer{w: w}
}

func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	if err := p.w.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
	}); err != nil {
		return errors.Wrap(err, "kafka publish")
	}
	return nil
}


