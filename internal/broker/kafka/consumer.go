package kafka

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/segmentio/kafka-go"
)

type messageReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type Consumer struct {
	r messageReader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	cfg := kafka.ReaderConfig{
		Brokers:           brokers,
		GroupID:           groupID,
		HeartbeatInterval: 3 * time.Second,
		SessionTimeout:    30 * time.Second,
	}
	if groupID != "" {
		cfg.GroupTopics = []string{topic}
	} else {
		cfg.Topic = topic
	}
	return &Consumer{
		r: kafka.NewReader(cfg),
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
		msg, err := c.r.FetchMessage(ctx)
		if err != nil {
			return errors.Wrap(err, "fetch message")
		}
		if err := handler(msg.Key, msg.Value); err != nil {
			// Важно: commit делаем только при успехе, иначе потеряем сообщение.
			return err
		}
		if err := c.r.CommitMessages(ctx, msg); err != nil {
			return errors.Wrap(err, "commit message")
		}
	}
}


