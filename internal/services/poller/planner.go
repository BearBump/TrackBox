package poller

import (
	"math/rand"
	"time"
)

type Rand interface {
	Intn(n int) int
}

type PlannerConfig struct {
	DeliveredDelay time.Duration // default: 365 days

	InTransitMinDelay time.Duration // default: 30 minutes
	InTransitMaxDelay time.Duration // default: 120 minutes

	UnknownDelay time.Duration // default: 90 minutes

	Backoff1 time.Duration // default: 5 minutes
	Backoff2 time.Duration // default: 15 minutes
	Backoff3 time.Duration // default: 30 minutes
	Backoff4 time.Duration // default: 60 minutes
}

func DefaultPlannerConfig() PlannerConfig {
	return PlannerConfig{
		DeliveredDelay: 365 * 24 * time.Hour,

		// Default: 1 minute. This can be overridden via config in track-worker.
		InTransitMinDelay: 1 * time.Minute,
		InTransitMaxDelay: 1 * time.Minute,

		UnknownDelay: 1 * time.Minute,

		Backoff1: 5 * time.Minute,
		Backoff2: 15 * time.Minute,
		Backoff3: 30 * time.Minute,
		Backoff4: 60 * time.Minute,
	}
}

type Planner struct {
	cfg PlannerConfig
	r   Rand
}

func NewPlanner(cfg PlannerConfig, r Rand) *Planner {
	def := DefaultPlannerConfig()
	if cfg.DeliveredDelay <= 0 {
		cfg.DeliveredDelay = def.DeliveredDelay
	}
	if cfg.InTransitMinDelay <= 0 {
		cfg.InTransitMinDelay = def.InTransitMinDelay
	}
	if cfg.InTransitMaxDelay <= 0 {
		cfg.InTransitMaxDelay = def.InTransitMaxDelay
	}
	if cfg.InTransitMaxDelay < cfg.InTransitMinDelay {
		cfg.InTransitMaxDelay = cfg.InTransitMinDelay
	}
	if cfg.UnknownDelay <= 0 {
		cfg.UnknownDelay = def.UnknownDelay
	}
	if cfg.Backoff1 <= 0 {
		cfg.Backoff1 = def.Backoff1
	}
	if cfg.Backoff2 <= 0 {
		cfg.Backoff2 = def.Backoff2
	}
	if cfg.Backoff3 <= 0 {
		cfg.Backoff3 = def.Backoff3
	}
	if cfg.Backoff4 <= 0 {
		cfg.Backoff4 = def.Backoff4
	}
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Planner{cfg: cfg, r: r}
}

func (p *Planner) NextCheckDelay(status string) time.Duration {
	switch status {
	case "DELIVERED":
		return p.cfg.DeliveredDelay
	case "IN_TRANSIT":
		min := p.cfg.InTransitMinDelay
		max := p.cfg.InTransitMaxDelay
		if max == min {
			return min
		}
		secMin := int(min.Seconds())
		secMax := int(max.Seconds())
		if secMin < 0 {
			secMin = 0
		}
		if secMax < secMin {
			secMax = secMin
		}
		return time.Duration(secMin+p.r.Intn(secMax-secMin+1)) * time.Second
	default:
		return p.cfg.UnknownDelay
	}
}

func (p *Planner) BackoffDelay(nextFailCount int32) time.Duration {
	switch {
	case nextFailCount <= 1:
		return p.cfg.Backoff1
	case nextFailCount == 2:
		return p.cfg.Backoff2
	case nextFailCount == 3:
		return p.cfg.Backoff3
	default:
		return p.cfg.Backoff4
	}
}

// Backward-compatible helpers (used in existing tests/code).
func NextCheckDelay(status string, _nextFailCount int32, r Rand) time.Duration {
	return NewPlanner(DefaultPlannerConfig(), r).NextCheckDelay(status)
}

func BackoffDelay(nextFailCount int32) time.Duration {
	return NewPlanner(DefaultPlannerConfig(), nil).BackoffDelay(nextFailCount)
}


