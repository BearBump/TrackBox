package kafka

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/segmentio/kafka-go"
)

type messageReader interface {
	ReadMessage(ctx context.Context) (kafka.Message, error)
	Close() error
}

type Consumer struct {
	r messageReader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers:           brokers,
			Topic:             topic,
			GroupID:           groupID,
			HeartbeatInterval: 3 * time.Second,
			SessionTimeout:    30 * time.Second,
		}),
	}
}

func newConsumerWithReader(r messageReader) *Consumer {
	return &Consumer{r: r}
}

func (c *Consumer) Close() error {
	return c.r.Close()
}

func (c *Consumer) Consume(ctx context.Context, handler func(key, value []byte) error) error {
	for {
		msg, err := c.r.ReadMessage(ctx)
		if err != nil {
			return errors.Wrap(err, "read message")
		}
		if err := handler(msg.Key, msg.Value); err != nil {
			// Reader коммитит автоматически после ReadMessage,
			// поэтому для MVP просто возвращаем ошибку и даём процессу рестартиться.
			return err
		}
	}
}


