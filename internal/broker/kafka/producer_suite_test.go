package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type writerMock struct {
	mock.Mock
}

func (m *writerMock) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	args := m.Called(ctx, msgs)
	return args.Error(0)
}

type ProducerSuite struct {
	suite.Suite
	wm *writerMock
	p  *Producer
}

func (s *ProducerSuite) SetupTest() {
	s.wm = &writerMock{}
	s.p = newProducerWithWriter(s.wm)
}

func (s *ProducerSuite) TestNewProducer_NotNil() {
	p := NewProducer([]string{"localhost:0"})
	s.Require().NotNil(p)
}

func (s *ProducerSuite) TestNewProducerWithWriter_NotNil() {
	p := newProducerWithWriter(s.wm)
	s.Require().NotNil(p)
}

func (s *ProducerSuite) TestPublish_OK() {
	s.wm.
		On("WriteMessages", mock.Anything, mock.MatchedBy(func(msgs []kafka.Message) bool {
			if len(msgs) != 1 {
				return false
			}
			return msgs[0].Topic == "t" && string(msgs[0].Key) == "k" && string(msgs[0].Value) == "v"
		})).
		Return(nil).
		Once()

	s.Require().NoError(s.p.Publish(context.Background(), "t", []byte("k"), []byte("v")))
	s.wm.AssertExpectations(s.T())
}

func (s *ProducerSuite) TestPublish_ErrorWrapped() {
	want := errors.New("boom")
	s.wm.On("WriteMessages", mock.Anything, mock.Anything).Return(want).Once()

	err := s.p.Publish(context.Background(), "t", []byte("k"), []byte("v"))
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "kafka publish")
	s.wm.AssertExpectations(s.T())
}

func TestProducerSuite(t *testing.T) {
	suite.Run(t, new(ProducerSuite))
}


