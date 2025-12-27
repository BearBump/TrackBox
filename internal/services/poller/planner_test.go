package poller

import (
	"testing"
	"time"

	pollermocks "github.com/BearBump/TrackBox/internal/services/poller/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type PlannerSuite struct {
	suite.Suite
}

func (s *PlannerSuite) TestBackoffDelay() {
	s.Equal(5*time.Minute, BackoffDelay(1))
	s.Equal(15*time.Minute, BackoffDelay(2))
	s.Equal(30*time.Minute, BackoffDelay(3))
	s.Equal(60*time.Minute, BackoffDelay(4))
	s.Equal(60*time.Minute, BackoffDelay(100))
}

func (s *PlannerSuite) TestNextCheckDelay_Delivered() {
	m := &pollermocks.Rand{}
	d := NextCheckDelay("DELIVERED", 0, m)
	s.Equal(365*24*time.Hour, d)
}

func (s *PlannerSuite) TestNextCheckDelay_InTransit_UsesRand() {
	// We'll force min==max==60s in default config; still ensure Rand can be passed safely.
	m := &pollermocks.Rand{}
	// Intn may or may not be called depending on min/max; just allow any call.
	m.On("Intn", mock.Anything).Return(0).Maybe()

	d := NextCheckDelay("IN_TRANSIT", 0, m)
	s.Equal(1*time.Minute, d)
	m.AssertExpectations(s.T())
}

func (s *PlannerSuite) TestNextCheckDelay_Unknown() {
	m := &pollermocks.Rand{}
	d := NextCheckDelay("UNKNOWN", 0, m)
	s.Equal(1*time.Minute, d)
}

func TestPlannerSuite(t *testing.T) {
	suite.Run(t, new(PlannerSuite))
}


