package trackings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/cache"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/pkg/errors"
)

type Repository interface {
	CreateOrGetTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error)
	GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error)
	ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error)
	RefreshTracking(ctx context.Context, trackingID uint64) error
	ApplyTrackingUpdate(ctx context.Context, upd pgtracking.TrackingUpdate) error
}

type Service struct {
	repo Repository
	cache cache.BytesCache
	currentTTL time.Duration
}

func New(repo Repository, c cache.BytesCache, currentTTL time.Duration) *Service {
	return &Service{repo: repo, cache: c, currentTTL: currentTTL}
}

func (s *Service) CreateTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error) {
	if len(items) == 0 {
		return nil, errors.New("items is empty")
	}
	if len(items) > 10_000 {
		return nil, errors.New("too many items (max 10000)")
	}

	clean := make([]models.TrackingCreateInput, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.CarrierCode == "" {
			return nil, errors.New("carrierCode is required")
		}
		if it.TrackNumber == "" {
			return nil, errors.New("trackNumber is required")
		}
		k := fmt.Sprintf("%s|%s", it.CarrierCode, it.TrackNumber)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		clean = append(clean, it)
	}

	return s.repo.CreateOrGetTrackings(ctx, clean)
}

func (s *Service) GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error) {
	if len(ids) == 0 {
		return []*models.Tracking{}, nil
	}
	// MVP: кэшируем "текущее состояние" целиком как JSON трекинга.
	// Для простоты делаем "лучшее усилие": кэш не обязан быть всегда.
	miss := make([]uint64, 0, len(ids))
	got := make(map[uint64]*models.Tracking, len(ids))

	if s.cache != nil && s.currentTTL > 0 {
		for _, id := range ids {
			key := currentKey(id)
			b, ok, err := s.cache.Get(ctx, key)
			if err != nil || !ok {
				miss = append(miss, id)
				continue
			}
			var t models.Tracking
			if json.Unmarshal(b, &t) != nil {
				miss = append(miss, id)
				continue
			}
			got[id] = &t
		}
	} else {
		miss = ids
	}

	var fromDB []*models.Tracking
	var err error
	if len(miss) > 0 {
		fromDB, err = s.repo.GetTrackingsByIDs(ctx, miss)
		if err != nil {
			return nil, err
		}
		if s.cache != nil && s.currentTTL > 0 {
			for _, t := range fromDB {
				b, _ := json.Marshal(t)
				_ = s.cache.Set(ctx, currentKey(t.ID), b, s.currentTTL)
			}
		}
		for _, t := range fromDB {
			got[t.ID] = t
		}
	}

	// Собираем ответ в том же порядке, что ids.
	out := make([]*models.Tracking, 0, len(ids))
	for _, id := range ids {
		if t, ok := got[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Service) ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error) {
	return s.repo.ListTrackingEvents(ctx, trackingID, limit, offset)
}

func (s *Service) RefreshTracking(ctx context.Context, trackingID uint64) error {
	if trackingID == 0 {
		return errors.New("trackingId is required")
	}
	return s.repo.RefreshTracking(ctx, trackingID)
}

func (s *Service) ApplyKafkaUpdate(ctx context.Context, msg messages.TrackingUpdated) error {
	if msg.TrackingID == 0 {
		return errors.New("tracking_id is required")
	}
	if msg.CheckedAt.IsZero() {
		msg.CheckedAt = time.Now().UTC()
	}
	if msg.NextCheckAt.IsZero() {
		// fallback: если воркер не послал next_check_at, ставим "через час"
		msg.NextCheckAt = msg.CheckedAt.Add(60 * time.Minute)
	}

	var events []*models.TrackingEvent
	for _, e := range msg.Events {
		var payloadStr *string
		if len(e.Payload) > 0 {
			s := string(e.Payload)
			payloadStr = &s
		}
		events = append(events, &models.TrackingEvent{
			Status:     e.Status,
			StatusRaw:  e.StatusRaw,
			EventTime:  e.EventTime,
			Location:   e.Location,
			Message:    e.Message,
			PayloadJSON: payloadStr,
		})
	}

	err := s.repo.ApplyTrackingUpdate(ctx, pgtracking.TrackingUpdate{
		TrackingID:  msg.TrackingID,
		CheckedAt:   msg.CheckedAt,
		Status:      msg.Status,
		StatusRaw:   msg.StatusRaw,
		StatusAt:    msg.StatusAt,
		NextCheckAt: msg.NextCheckAt,
		Events:      events,
		Error:       msg.Error,
	})
	if err != nil {
		return err
	}

	// Инвалидируем/обновляем кэш текущего статуса.
	if s.cache != nil && s.currentTTL > 0 {
		// Просто перезагрузим из БД одну запись.
		ts, err := s.repo.GetTrackingsByIDs(ctx, []uint64{msg.TrackingID})
		if err == nil && len(ts) == 1 {
			b, _ := json.Marshal(ts[0])
			_ = s.cache.Set(ctx, currentKey(msg.TrackingID), b, s.currentTTL)
		}
	}

	return nil
}

func currentKey(id uint64) string {
	return fmt.Sprintf("tracking:%d:current", id)
}


