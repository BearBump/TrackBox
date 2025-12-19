package emulatorv1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/pkg/errors"
)

type Client struct {
	baseURL string
	apiKey  string
	httpc   *http.Client
}

func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:9000"
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpc: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type respEvent struct {
	Status    string     `json:"status"`
	StatusRaw string     `json:"status_raw"`
	EventTime time.Time  `json:"event_time"`
	Location  *string    `json:"location,omitempty"`
	Message   *string    `json:"message,omitempty"`
	Payload   any        `json:"payload,omitempty"`
}

type respBody struct {
	TrackingID  string      `json:"tracking_id,omitempty"`
	Carrier     string      `json:"carrier"`
	TrackNumber string      `json:"track_number"`
	Status      string      `json:"status"`
	StatusRaw   string      `json:"status_raw"`
	StatusAt    time.Time   `json:"status_at"`
	Events      []respEvent `json:"events"`
}

func (c *Client) GetTracking(ctx context.Context, carrierCode, trackNumber string) (carrier.TrackingResult, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "parse base url")
	}
	u.Path = fmt.Sprintf("/v1/tracking/%s/%s", url.PathEscape(carrierCode), url.PathEscape(trackNumber))
	q := u.Query()
	if c.apiKey != "" {
		q.Set("apiKey", c.apiKey)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "new request")
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "do request")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return carrier.TrackingResult{}, fmt.Errorf("carrier emulator rate limit (429)")
	}
	if resp.StatusCode/100 != 2 {
		return carrier.TrackingResult{}, fmt.Errorf("carrier emulator http %d", resp.StatusCode)
	}

	var rb respBody
	if err := json.NewDecoder(resp.Body).Decode(&rb); err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "decode")
	}

	status := rb.Status
	if status == "" {
		status = models.TrackingStatusUnknown
	}

	var evs []*models.TrackingEvent
	for _, e := range rb.Events {
		evs = append(evs, &models.TrackingEvent{
			Status:    e.Status,
			StatusRaw: e.StatusRaw,
			EventTime: e.EventTime,
			Location:  e.Location,
			Message:   e.Message,
		})
	}

	statusAt := rb.StatusAt
	return carrier.TrackingResult{
		Status:    status,
		StatusRaw: rb.StatusRaw,
		StatusAt:  &statusAt,
		Events:    evs,
	}, nil
}


