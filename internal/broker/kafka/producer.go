package kafka

import (
	"context"

	"github.com/pkg/errors"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	w *kafka.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
		w: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Balancer: &kafka.LeastBytes{},
		},
	}
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


