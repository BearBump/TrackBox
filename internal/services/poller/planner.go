package poller

import (
	"math/rand"
	"time"
)

type Rand interface {
	Intn(n int) int
}

// NextCheckDelay реализует правила из ТЗ:
// - DELIVERED -> очень редко (условно 365 дней)
// - IN_TRANSIT -> 30..120 минут
// - ошибка -> backoff 5/15/30/60 минут по номеру фейла
func NextCheckDelay(status string, nextFailCount int32, r Rand) time.Duration {
	switch status {
	case "DELIVERED":
		return 365 * 24 * time.Hour
	case "IN_TRANSIT":
		min := 30
		max := 120
		if r == nil {
			r = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		return time.Duration(min+r.Intn(max-min+1)) * time.Minute
	default:
		// UNKNOWN и всё остальное — как "в пути" чуть реже
		return 90 * time.Minute
	}
}

func BackoffDelay(nextFailCount int32) time.Duration {
	switch {
	case nextFailCount <= 1:
		return 5 * time.Minute
	case nextFailCount == 2:
		return 15 * time.Minute
	case nextFailCount == 3:
		return 30 * time.Minute
	default:
		return 60 * time.Minute
	}
}


