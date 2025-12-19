package track24http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/pkg/errors"
)

type Client struct {
	baseURL string
	apiKey  string
	domain  string
	httpc   *http.Client
}

func New(baseURL, apiKey, domain string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:9000"
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		domain:  domain,
		httpc: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type track24Resp struct {
	Status string `json:"status"`
	Data   struct {
		Events []struct {
			OperationDateTime        string `json:"operationDateTime"`
			OperationAttribute       string `json:"operationAttribute"`
			OperationType            string `json:"operationType"`
			OperationPlaceName       string `json:"operationPlaceName"`
			OperationPlacePostalCode string `json:"operationPlacePostalCode"`
			Source                   string `json:"source"`
		} `json:"events"`
	} `json:"data"`
}

func (c *Client) GetTracking(ctx context.Context, carrierCode, trackNumber string) (carrier.TrackingResult, error) {
	_ = carrierCode // в Track24 запросе carrier обычно автоопределяется

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "parse base url")
	}
	u.Path = "/tracking.json.php"

	q := u.Query()
	q.Set("apiKey", c.apiKey)
	q.Set("domain", c.domain)
	q.Set("code", trackNumber)
	q.Set("pretty", "true")
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

	if resp.StatusCode/100 != 2 {
		return carrier.TrackingResult{}, fmt.Errorf("track24 emulator http %d", resp.StatusCode)
	}

	var r track24Resp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return carrier.TrackingResult{}, errors.Wrap(err, "decode")
	}
	if r.Status != "ok" {
		return carrier.TrackingResult{}, fmt.Errorf("track24 emulator status=%s", r.Status)
	}

	// Простейшая нормализация: если последняя операция содержит "вруч" или "достав" -> DELIVERED, иначе IN_TRANSIT.
	now := time.Now().UTC()
	status := models.TrackingStatusInTransit
	statusRaw := ""
	var events []*models.TrackingEvent

	for _, e := range r.Data.Events {
		msg := e.OperationAttribute
		loc := e.OperationPlaceName
		statusRaw = msg

		evTime := now
		// Track24 пример: "02.07.2014 19:16:00"
		if e.OperationDateTime != "" {
			if t, err := time.ParseInLocation("02.01.2006 15:04:05", e.OperationDateTime, time.UTC); err == nil {
				evTime = t.UTC()
			}
		}

		events = append(events, &models.TrackingEvent{
			Status:    status,
			StatusRaw: msg,
			EventTime: evTime,
			Location:  strPtr(loc),
			Message:   strPtr(msg),
		})
	}

	if len(r.Data.Events) > 0 {
		last := r.Data.Events[len(r.Data.Events)-1]
		la := last.OperationAttribute
		if containsDeliveredHint(la) {
			status = models.TrackingStatusDelivered
		}
	}

	return carrier.TrackingResult{
		Status:    status,
		StatusRaw: statusRaw,
		StatusAt:  &now,
		Events:    events,
	}, nil
}

func containsDeliveredHint(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "вруч") || strings.Contains(low, "достав") || strings.Contains(low, "delivered")
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}


