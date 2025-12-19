package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/pkg/errors"
)

type Repository interface {
	ClaimDueTrackings(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]*models.Tracking, error)
}

type Producer interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error)
}

type Poller struct {
	repo Repository
	carrier carrier.Client
	producer Producer
	rl RateLimiter

	topic string

	pollInterval time.Duration
	batchSize int
	concurrency int
	lease time.Duration
	rateLimitPerMinute int64
	rateLimitCDEKPerMinute int64
	rateLimitPostRuPerMinute int64
}

func New(repo Repository, carrier carrier.Client, producer Producer, rl RateLimiter, topic string) *Poller {
	return &Poller{
		repo: repo, carrier: carrier, producer: producer, rl: rl, topic: topic,
		pollInterval: 2 * time.Second,
		batchSize: 100,
		concurrency: 10,
		lease: 120 * time.Second,
		rateLimitPerMinute: 120,
	}
}

func (p *Poller) WithSettings(pollInterval time.Duration, batchSize, concurrency int, lease time.Duration, rlPerMin int64) *Poller {
	if pollInterval > 0 {
		p.pollInterval = pollInterval
	}
	if batchSize > 0 {
		p.batchSize = batchSize
	}
	if concurrency > 0 {
		p.concurrency = concurrency
	}
	if lease > 0 {
		p.lease = lease
	}
	if rlPerMin > 0 {
		p.rateLimitPerMinute = rlPerMin
	}
	return p
}

func (p *Poller) WithCarrierRateLimits(cdekPerMin, postRuPerMin int) *Poller {
	if cdekPerMin > 0 {
		p.rateLimitCDEKPerMinute = int64(cdekPerMin)
	}
	if postRuPerMin > 0 {
		p.rateLimitPostRuPerMinute = int64(postRuPerMin)
	}
	return p
}

func (p *Poller) Run(ctx context.Context) error {
	sem := make(chan struct{}, p.concurrency)
	t := time.NewTicker(p.pollInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			now := time.Now().UTC()
			items, err := p.repo.ClaimDueTrackings(ctx, now, p.batchSize, p.lease)
			if err != nil {
				slog.Error("claim due trackings", "error", err.Error())
				continue
			}
			for _, tr := range items {
				sem <- struct{}{}
				trCopy := tr
				go func() {
					defer func() { <-sem }()
					if err := p.processOne(ctx, trCopy); err != nil {
						slog.Error("process tracking", "tracking_id", trCopy.ID, "error", err.Error())
					}
				}()
			}
		}
	}
}

func (p *Poller) processOne(ctx context.Context, tr *models.Tracking) error {
	now := time.Now().UTC()

	if p.rl != nil && p.rateLimitPerMinute > 0 {
		limit := p.rateLimitPerMinute
		switch tr.CarrierCode {
		case "CDEK":
			if p.rateLimitCDEKPerMinute > 0 {
				limit = p.rateLimitCDEKPerMinute
			}
		case "POST_RU":
			if p.rateLimitPostRuPerMinute > 0 {
				limit = p.rateLimitPostRuPerMinute
			}
		}

		minuteKey := fmt.Sprintf("rl:carrier:%s:%s", tr.CarrierCode, now.Format("200601021504"))
		allowed, n, err := p.rl.Allow(ctx, minuteKey, limit, 70*time.Second)
		if err != nil {
			return err
		}
		if !allowed {
			// Слишком много запросов в минуту: подождём немного, чтобы разгрузить источник.
			slog.Warn("rate limit exceeded", "carrier", tr.CarrierCode, "count", n)
			time.Sleep(500 * time.Millisecond)
		}
	}

	res, err := p.carrier.GetTracking(ctx, tr.CarrierCode, tr.TrackNumber)
	msg := messages.TrackingUpdated{
		TrackingID: tr.ID,
		CheckedAt:  now,
	}

	if err != nil {
		e := err.Error()
		msg.Error = &e
		nextFail := tr.CheckFailCount + 1
		msg.NextCheckAt = now.Add(BackoffDelay(nextFail))
	} else {
		msg.Status = res.Status
		msg.StatusRaw = res.StatusRaw
		msg.StatusAt = res.StatusAt
		msg.NextCheckAt = now.Add(NextCheckDelay(res.Status, tr.CheckFailCount, nil))
		for _, e := range res.Events {
			var payload json.RawMessage
			if e.PayloadJSON != nil && *e.PayloadJSON != "" {
				payload = json.RawMessage(*e.PayloadJSON)
			}
			msg.Events = append(msg.Events, messages.TrackingEvent{
				Status:    e.Status,
				StatusRaw: e.StatusRaw,
				EventTime: e.EventTime,
				Location:  e.Location,
				Message:   e.Message,
				Payload:   payload,
			})
		}
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "marshal kafka msg")
	}

	key := []byte(fmt.Sprintf("%d", tr.ID))
	if err := p.producer.Publish(ctx, p.topic, key, b); err != nil {
		return err
	}
	return nil
}


