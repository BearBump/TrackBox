package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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

	planner *Planner

	pollInterval time.Duration
	batchSize int
	concurrency int
	lease time.Duration
	rateLimitPerMinute int64
	rateLimitCDEKPerMinute int64
	rateLimitPostRuPerMinute int64

	triggerCh chan struct{}

	startedAtUnixNano   int64
	lastCycleUnixNano   atomic.Int64
	lastTriggerUnixNano atomic.Int64
	totalClaimed        atomic.Int64
	totalProcessed      atomic.Int64
	totalErrors         atomic.Int64
	inFlight            atomic.Int64
	lastErrorMu         sync.Mutex
	lastError           string
}

func New(repo Repository, carrier carrier.Client, producer Producer, rl RateLimiter, topic string) *Poller {
	return &Poller{
		repo: repo, carrier: carrier, producer: producer, rl: rl, topic: topic,
		planner: DefaultPlanner(),
		pollInterval: 2 * time.Second,
		batchSize: 100,
		concurrency: 10,
		lease: 120 * time.Second,
		rateLimitPerMinute: 120,
		triggerCh: make(chan struct{}, 1),
		startedAtUnixNano: time.Now().UTC().UnixNano(),
	}
}

func DefaultPlanner() *Planner {
	return NewPlanner(DefaultPlannerConfig(), nil)
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

func (p *Poller) WithPlanner(cfg PlannerConfig) *Poller {
	p.planner = NewPlanner(cfg, nil)
	return p
}

// Trigger forces an immediate poll cycle (best-effort, non-blocking).
func (p *Poller) Trigger() {
	p.lastTriggerUnixNano.Store(time.Now().UTC().UnixNano())
	select {
	case p.triggerCh <- struct{}{}:
	default:
	}
}

type Stats struct {
	StartedAt      time.Time `json:"startedAt"`
	LastCycleAt    *time.Time `json:"lastCycleAt,omitempty"`
	LastTriggerAt  *time.Time `json:"lastTriggerAt,omitempty"`
	TotalClaimed   int64     `json:"totalClaimed"`
	TotalProcessed int64     `json:"totalProcessed"`
	TotalErrors    int64     `json:"totalErrors"`
	InFlight       int64     `json:"inFlight"`
	LastError      string    `json:"lastError,omitempty"`
}

func (p *Poller) Stats() Stats {
	st := Stats{
		StartedAt:      time.Unix(0, p.startedAtUnixNano).UTC(),
		TotalClaimed:   p.totalClaimed.Load(),
		TotalProcessed: p.totalProcessed.Load(),
		TotalErrors:    p.totalErrors.Load(),
		InFlight:       p.inFlight.Load(),
	}
	if n := p.lastCycleUnixNano.Load(); n > 0 {
		t := time.Unix(0, n).UTC()
		st.LastCycleAt = &t
	}
	if n := p.lastTriggerUnixNano.Load(); n > 0 {
		t := time.Unix(0, n).UTC()
		st.LastTriggerAt = &t
	}
	p.lastErrorMu.Lock()
	st.LastError = p.lastError
	p.lastErrorMu.Unlock()
	return st
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
	t := time.NewTicker(p.pollInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			p.runOnce(ctx)
		case <-p.triggerCh:
			p.runOnce(ctx)
		}
	}
}

func (p *Poller) runOnce(ctx context.Context) {
	now := time.Now().UTC()
	p.lastCycleUnixNano.Store(now.UnixNano())

	items, err := p.repo.ClaimDueTrackings(ctx, now, p.batchSize, p.lease)
	if err != nil {
		slog.Error("claim due trackings", "error", err.Error())
		p.lastErrorMu.Lock()
		p.lastError = err.Error()
		p.lastErrorMu.Unlock()
		return
	}
	p.totalClaimed.Add(int64(len(items)))

	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup
	for _, tr := range items {
		sem <- struct{}{}
		wg.Add(1)
		trCopy := tr
		p.inFlight.Add(1)
		go func() {
			defer func() {
				p.inFlight.Add(-1)
				<-sem
				wg.Done()
			}()
			if err := p.processOne(ctx, trCopy); err != nil {
				p.totalErrors.Add(1)
				p.lastErrorMu.Lock()
				p.lastError = err.Error()
				p.lastErrorMu.Unlock()
				slog.Error("process tracking", "tracking_id", trCopy.ID, "error", err.Error())
			}
			p.totalProcessed.Add(1)
		}()
	}
	wg.Wait()
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
		msg.NextCheckAt = now.Add(p.planner.BackoffDelay(nextFail))
	} else {
		msg.Status = res.Status
		msg.StatusRaw = res.StatusRaw
		msg.StatusAt = res.StatusAt
		msg.NextCheckAt = now.Add(p.planner.NextCheckDelay(res.Status))
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
	// Kafka может быть не готова сразу после старта docker compose.
	// Для демо/устойчивости делаем небольшой retry.
	var pubErr error
	for i := 0; i < 10; i++ {
		if err := p.producer.Publish(ctx, p.topic, key, b); err == nil {
			pubErr = nil
			break
		} else {
			pubErr = err
			time.Sleep(time.Duration(150*(i+1)) * time.Millisecond)
		}
	}
	if pubErr != nil {
		return pubErr
	}
	return nil
}


