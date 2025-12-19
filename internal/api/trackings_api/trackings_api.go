package trackings_api

import (
	"context"

	"trackbox/internal/models"
	pb_models "trackbox/internal/pb/models"
	"trackbox/internal/pb/trackings_api"
	"trackbox/internal/services/trackings"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TrackingsAPI struct {
	trackings_api.UnimplementedTrackingsServiceServer
	svc *trackings.Service
}

func New(svc *trackings.Service) *TrackingsAPI {
	return &TrackingsAPI{svc: svc}
}

func (a *TrackingsAPI) CreateTrackings(ctx context.Context, req *trackings_api.CreateTrackingsRequest) (*trackings_api.CreateTrackingsResponse, error) {
	in := make([]models.TrackingCreateInput, 0, len(req.GetItems()))
	for _, it := range req.GetItems() {
		in = append(in, models.TrackingCreateInput{
			CarrierCode: it.GetCarrierCode(),
			TrackNumber: it.GetTrackNumber(),
		})
	}
	ts, err := a.svc.CreateTrackings(ctx, in)
	if err != nil {
		return nil, err
	}
	return &trackings_api.CreateTrackingsResponse{Trackings: toPBTrackings(ts)}, nil
}

func (a *TrackingsAPI) GetTrackingsByIds(ctx context.Context, req *trackings_api.GetTrackingsByIdsRequest) (*trackings_api.GetTrackingsByIdsResponse, error) {
	ts, err := a.svc.GetTrackingsByIDs(ctx, req.GetIds())
	if err != nil {
		return nil, err
	}
	return &trackings_api.GetTrackingsByIdsResponse{Trackings: toPBTrackings(ts)}, nil
}

func (a *TrackingsAPI) ListTrackingEvents(ctx context.Context, req *trackings_api.ListTrackingEventsRequest) (*trackings_api.ListTrackingEventsResponse, error) {
	evs, err := a.svc.ListTrackingEvents(ctx, req.GetTrackingId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*pb_models.TrackingEvent, 0, len(evs))
	for _, e := range evs {
		out = append(out, &pb_models.TrackingEvent{
			Id:         e.ID,
			TrackingId: e.TrackingID,
			Status:     e.Status,
			StatusRaw:  e.StatusRaw,
			EventTime:  timestamppb.New(e.EventTime),
			Location:   derefString(e.Location),
			Message:    derefString(e.Message),
			PayloadJson: derefString(e.PayloadJSON),
			CreatedAt:  timestamppb.New(e.CreatedAt),
		})
	}
	return &trackings_api.ListTrackingEventsResponse{Events: out}, nil
}

func (a *TrackingsAPI) RefreshTracking(ctx context.Context, req *trackings_api.RefreshTrackingRequest) (*emptypb.Empty, error) {
	if err := a.svc.RefreshTracking(ctx, req.GetTrackingId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func toPBTrackings(ts []*models.Tracking) []*pb_models.Tracking {
	out := make([]*pb_models.Tracking, 0, len(ts))
	for _, t := range ts {
		var statusAt *timestamppb.Timestamp
		if t.StatusAt != nil {
			statusAt = timestamppb.New(*t.StatusAt)
		}
		var lastCheckedAt *timestamppb.Timestamp
		if t.LastCheckedAt != nil {
			lastCheckedAt = timestamppb.New(*t.LastCheckedAt)
		}
		out = append(out, &pb_models.Tracking{
			Id:            t.ID,
			CarrierCode:   t.CarrierCode,
			TrackNumber:   t.TrackNumber,
			Status:        t.Status,
			StatusRaw:     t.StatusRaw,
			StatusAt:      statusAt,
			LastCheckedAt: lastCheckedAt,
			NextCheckAt:   timestamppb.New(t.NextCheckAt),
			CheckFailCount: t.CheckFailCount,
			LastError:     derefString(t.LastError),
			CreatedAt:     timestamppb.New(t.CreatedAt),
			UpdatedAt:     timestamppb.New(t.UpdatedAt),
		})
	}
	return out
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}


