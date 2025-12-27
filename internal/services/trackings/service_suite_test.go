package trackings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	cachemocks "github.com/BearBump/TrackBox/internal/cache/mocks"
	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	trackingsmocks "github.com/BearBump/TrackBox/internal/services/trackings/mocks"
)

type ServiceSuite struct {
	suite.Suite

	repo  *trackingsmocks.MockRepository
	cache *cachemocks.MockBytesCache
	svc   *Service
}

func (s *ServiceSuite) SetupTest() {
	s.repo = &trackingsmocks.MockRepository{}
	s.cache = &cachemocks.MockBytesCache{}
	s.svc = New(s.repo, s.cache, 10*time.Minute)
}

func (s *ServiceSuite) TestCreateTrackings_DedupAndCallsRepo() {
	in := []models.TrackingCreateInput{
		{CarrierCode: "CDEK", TrackNumber: "A"},
		{CarrierCode: "CDEK", TrackNumber: "A"},
		{CarrierCode: "POST_RU", TrackNumber: "B"},
	}
	wantRepoIn := []models.TrackingCreateInput{
		{CarrierCode: "CDEK", TrackNumber: "A"},
		{CarrierCode: "POST_RU", TrackNumber: "B"},
	}
	s.repo.On("CreateOrGetTrackings", mock.Anything, wantRepoIn).
		Return([]*models.Tracking{{ID: 1}, {ID: 2}}, nil).
		Once()

	out, err := s.svc.CreateTrackings(context.Background(), in)
	s.Require().NoError(err)
	s.Require().Len(out, 2)
	s.repo.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestCreateTrackings_ValidateErrors() {
	_, err := s.svc.CreateTrackings(context.Background(), nil)
	s.Require().Error(err)

	_, err = s.svc.CreateTrackings(context.Background(), []models.TrackingCreateInput{{CarrierCode: "", TrackNumber: "X"}})
	s.Require().Error(err)

	_, err = s.svc.CreateTrackings(context.Background(), []models.TrackingCreateInput{{CarrierCode: "C", TrackNumber: ""}})
	s.Require().Error(err)

	// too many items
	items := make([]models.TrackingCreateInput, 10_001)
	for i := range items {
		items[i] = models.TrackingCreateInput{CarrierCode: "C", TrackNumber: "N"}
	}
	_, err = s.svc.CreateTrackings(context.Background(), items)
	s.Require().Error(err)

	s.repo.AssertNotCalled(s.T(), "CreateOrGetTrackings", mock.Anything, mock.Anything)
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CacheHit_NoDB() {
	t := &models.Tracking{ID: 7, CarrierCode: "C", TrackNumber: "N", Status: models.TrackingStatusUnknown}
	b, _ := json.Marshal(t)

	s.cache.On("Get", mock.Anything, "tracking:7:current").
		Return(b, true, nil).
		Once()

	out, err := s.svc.GetTrackingsByIDs(context.Background(), []uint64{7})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.Require().Equal(uint64(7), out[0].ID)

	// DB не должен трогаться
	s.repo.AssertNotCalled(s.T(), "GetTrackingsByIDs", mock.Anything, mock.Anything)
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_EmptyIDs() {
	out, err := s.svc.GetTrackingsByIDs(context.Background(), nil)
	s.Require().NoError(err)
	s.Require().Len(out, 0)
	s.repo.AssertNotCalled(s.T(), "GetTrackingsByIDs", mock.Anything, mock.Anything)
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CacheDisabled_GoesToDB() {
	svc := New(s.repo, nil, 0)
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1), uint64(2)}).
		Return([]*models.Tracking{{ID: 1}, {ID: 2}}, nil).
		Once()

	out, err := svc.GetTrackingsByIDs(context.Background(), []uint64{1, 2})
	s.Require().NoError(err)
	s.Require().Len(out, 2)
	s.repo.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CachePresentButTTLZero_TreatedAsDisabled() {
	// cache есть, но TTL=0 => кэш выключен, Get/Set не должны вызываться
	svc := New(s.repo, s.cache, 0)
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1)}).
		Return([]*models.Tracking{{ID: 1}}, nil).
		Once()

	out, err := svc.GetTrackingsByIDs(context.Background(), []uint64{1})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.cache.AssertNotCalled(s.T(), "Get", mock.Anything, mock.Anything)
	s.cache.AssertNotCalled(s.T(), "Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.repo.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CacheMiss_AndSetEvenIfSetFails_OrderPreserved() {
	ids := []uint64{2, 1}
	s.cache.On("Get", mock.Anything, "tracking:2:current").
		Return([]byte(nil), false, nil).
		Once()
	s.cache.On("Get", mock.Anything, "tracking:1:current").
		Return([]byte(nil), false, nil).
		Once()

	// DB вернёт в другом порядке — сервис должен вернуть в порядке ids
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(2), uint64(1)}).
		Return([]*models.Tracking{{ID: 1}, {ID: 2}}, nil).
		Once()

	// Set ошибки игнорируются
	s.cache.On("Set", mock.Anything, "tracking:1:current", mock.Anything, 10*time.Minute).
		Return(errors.New("set failed")).
		Once()
	s.cache.On("Set", mock.Anything, "tracking:2:current", mock.Anything, 10*time.Minute).
		Return(errors.New("set failed")).
		Once()

	out, err := s.svc.GetTrackingsByIDs(context.Background(), ids)
	s.Require().NoError(err)
	s.Require().Len(out, 2)
	s.Require().Equal(uint64(2), out[0].ID)
	s.Require().Equal(uint64(1), out[1].ID)
	s.repo.AssertExpectations(s.T())
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CacheHitButEmptyBytes_IsMiss() {
	// cache ok=true, но bytes пустые -> json.Unmarshal падает -> идём в БД
	s.cache.On("Get", mock.Anything, "tracking:1:current").
		Return([]byte(nil), true, nil).
		Once()

	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1)}).
		Return([]*models.Tracking{{ID: 1}}, nil).
		Once()
	s.cache.On("Set", mock.Anything, "tracking:1:current", mock.Anything, 10*time.Minute).Return(nil).Once()

	out, err := s.svc.GetTrackingsByIDs(context.Background(), []uint64{1})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.repo.AssertExpectations(s.T())
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_CacheGetError_AndCacheBadJSON_BothMiss() {
	// 1) cache get error -> miss
	s.cache.On("Get", mock.Anything, "tracking:1:current").
		Return([]byte(nil), false, errors.New("redis down")).
		Once()
	// 2) cache ok but bad json -> miss
	s.cache.On("Get", mock.Anything, "tracking:2:current").
		Return([]byte("not-json"), true, nil).
		Once()

	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1), uint64(2)}).
		Return([]*models.Tracking{{ID: 1}, {ID: 2}}, nil).
		Once()
	s.cache.On("Set", mock.Anything, "tracking:1:current", mock.Anything, 10*time.Minute).Return(nil).Once()
	s.cache.On("Set", mock.Anything, "tracking:2:current", mock.Anything, 10*time.Minute).Return(nil).Once()

	out, err := s.svc.GetTrackingsByIDs(context.Background(), []uint64{1, 2})
	s.Require().NoError(err)
	s.Require().Len(out, 2)
	s.repo.AssertExpectations(s.T())
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestGetTrackingsByIDs_DBError() {
	s.cache.On("Get", mock.Anything, "tracking:1:current").
		Return([]byte(nil), false, nil).
		Once()
	want := errors.New("db error")
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1)}).
		Return([]*models.Tracking(nil), want).
		Once()
	_, err := s.svc.GetTrackingsByIDs(context.Background(), []uint64{1})
	s.Require().ErrorIs(err, want)
}

func (s *ServiceSuite) TestListTrackingEvents_Passthrough() {
	evs := []*models.TrackingEvent{{ID: 1, TrackingID: 9}}
	s.repo.On("ListTrackingEvents", mock.Anything, uint64(9), 50, 10).Return(evs, nil).Once()
	out, err := s.svc.ListTrackingEvents(context.Background(), 9, 50, 10)
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.repo.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestRefreshTracking_ValidateAndPass() {
	s.Require().Error(s.svc.RefreshTracking(context.Background(), 0))

	s.repo.On("RefreshTracking", mock.Anything, uint64(12)).Return(nil).Once()
	s.Require().NoError(s.svc.RefreshTracking(context.Background(), 12))
	s.repo.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestApplyKafkaUpdate_CallsRepoAndUpdatesCache() {
	now := time.Now().UTC()
	msg := messages.TrackingUpdated{
		TrackingID:  10,
		CheckedAt:   now,
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		NextCheckAt: now.Add(5 * time.Minute),
	}

	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.AnythingOfType("pgtracking.TrackingUpdate")).
		Return(nil).
		Once()
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(10)}).
		Return([]*models.Tracking{{ID: 10, CarrierCode: "C", TrackNumber: "N", Status: models.TrackingStatusInTransit}}, nil).
		Once()
	s.cache.On("Set", mock.Anything, "tracking:10:current", mock.Anything, 10*time.Minute).
		Return(nil).
		Once()

	s.Require().NoError(s.svc.ApplyKafkaUpdate(context.Background(), msg))
	s.repo.AssertExpectations(s.T())
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestApplyKafkaUpdate_ValidateTrackingID() {
	err := s.svc.ApplyKafkaUpdate(context.Background(), messages.TrackingUpdated{})
	s.Require().Error(err)
	s.repo.AssertNotCalled(s.T(), "ApplyTrackingUpdate", mock.Anything, mock.Anything)
}

func (s *ServiceSuite) TestApplyKafkaUpdate_DefaultTimes_EventsPayload_AndCacheReloadBranches() {
	// 1) first: apply ok, cache reload error -> Set не вызывается
	msg := messages.TrackingUpdated{
		TrackingID: 1,
		Status:     models.TrackingStatusInTransit,
		StatusRaw:  "RAW",
		// CheckedAt и NextCheckAt пустые -> сервис выставит сам
		Events: []messages.TrackingEvent{
			{Status: models.TrackingStatusInTransit, StatusRaw: "raw", EventTime: time.Now().UTC(), Payload: []byte(`{"x":1}`)},
		},
	}

	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.MatchedBy(func(upd pgtracking.TrackingUpdate) bool {
		if upd.TrackingID != 1 {
			return false
		}
		if upd.CheckedAt.IsZero() {
			return false
		}
		if upd.NextCheckAt.Sub(upd.CheckedAt) != 60*time.Minute {
			return false
		}
		if upd.Status != models.TrackingStatusInTransit || upd.StatusRaw != "RAW" {
			return false
		}
		if len(upd.Events) != 1 {
			return false
		}
		if upd.Events[0].PayloadJSON == nil || *upd.Events[0].PayloadJSON != `{"x":1}` {
			return false
		}
		return true
	})).Return(nil).Once()

	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(1)}).
		Return([]*models.Tracking(nil), errors.New("reload fail")).
		Once()
	s.cache.AssertNotCalled(s.T(), "Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.Require().NoError(s.svc.ApplyKafkaUpdate(context.Background(), msg))
	s.repo.AssertExpectations(s.T())

	// 2) second: reload returns len != 1 -> Set не вызывается
	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.AnythingOfType("pgtracking.TrackingUpdate")).Return(nil).Once()
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(2)}).
		Return([]*models.Tracking{{ID: 2}, {ID: 3}}, nil).
		Once()
	s.Require().NoError(s.svc.ApplyKafkaUpdate(context.Background(), messages.TrackingUpdated{
		TrackingID:  2,
		CheckedAt:   time.Now().UTC(),
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		NextCheckAt: time.Now().UTC().Add(1 * time.Minute),
	}))
}

func (s *ServiceSuite) TestApplyKafkaUpdate_EmptyPayload_AndLocationMessageBranches() {
	loc := "Moscow"
	msgText := "Accepted"
	evTime := time.Now().UTC()

	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.MatchedBy(func(upd pgtracking.TrackingUpdate) bool {
		if len(upd.Events) != 1 {
			return false
		}
		if upd.Events[0].Location == nil || *upd.Events[0].Location != loc {
			return false
		}
		if upd.Events[0].Message == nil || *upd.Events[0].Message != msgText {
			return false
		}
		// payload empty => PayloadJSON должен быть nil
		if upd.Events[0].PayloadJSON != nil {
			return false
		}
		if !upd.Events[0].EventTime.Equal(evTime) {
			return false
		}
		return true
	})).Return(nil).Once()

	// cache reload ok => Set вызывается
	s.repo.On("GetTrackingsByIDs", mock.Anything, []uint64{uint64(3)}).
		Return([]*models.Tracking{{ID: 3}}, nil).
		Once()
	s.cache.On("Set", mock.Anything, "tracking:3:current", mock.Anything, 10*time.Minute).Return(nil).Once()

	s.Require().NoError(s.svc.ApplyKafkaUpdate(context.Background(), messages.TrackingUpdated{
		TrackingID:  3,
		CheckedAt:   time.Now().UTC(),
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		NextCheckAt: time.Now().UTC().Add(1 * time.Minute),
		Events: []messages.TrackingEvent{
			{Status: models.TrackingStatusInTransit, StatusRaw: "CDEK: accepted", EventTime: evTime, Location: &loc, Message: &msgText},
		},
	}))

	s.repo.AssertExpectations(s.T())
	s.cache.AssertExpectations(s.T())
}

func (s *ServiceSuite) TestApplyKafkaUpdate_RepoErrorStops() {
	want := errors.New("apply failed")
	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.Anything).Return(want).Once()
	err := s.svc.ApplyKafkaUpdate(context.Background(), messages.TrackingUpdated{
		TrackingID:  99,
		CheckedAt:   time.Now().UTC(),
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		NextCheckAt: time.Now().UTC().Add(1 * time.Minute),
	})
	s.Require().ErrorIs(err, want)
}

func (s *ServiceSuite) TestApplyKafkaUpdate_NoCache_NoReload() {
	svc := New(s.repo, nil, 0)
	s.repo.On("ApplyTrackingUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	s.Require().NoError(svc.ApplyKafkaUpdate(context.Background(), messages.TrackingUpdated{
		TrackingID:  5,
		CheckedAt:   time.Now().UTC(),
		Status:      models.TrackingStatusInTransit,
		StatusRaw:   "RAW",
		NextCheckAt: time.Now().UTC().Add(1 * time.Minute),
	}))
	// reload не должен вызываться (cache nil/ttl=0)
	s.repo.AssertNotCalled(s.T(), "GetTrackingsByIDs", mock.Anything, []uint64{uint64(5)})
}

func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceSuite))
}


