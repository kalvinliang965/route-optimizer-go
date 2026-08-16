// Package matrix contains travel-cost matrix providers.
package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"route-optimizer-go/internal/optimizer"
)

const DefaultOSRMBaseURL = "http://router.project-osrm.org"

type OSRM struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
}

func NewOSRM(baseURL, userAgent string, timeout time.Duration) *OSRM {
	if baseURL == "" {
		baseURL = DefaultOSRMBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &OSRM{
		BaseURL:    baseURL,
		UserAgent:  userAgent,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (c *OSRM) Durations(ctx context.Context, stops []optimizer.Stop) (optimizer.Matrix, error) {
	if len(stops) == 0 {
		return nil, fmt.Errorf("at least one stop is required")
	}
	if len(stops) == 1 {
		return optimizer.Matrix{{0}}, nil
	}

	coordinates := make([]string, len(stops))
	for index, stop := range stops {
		coordinates[index] = fmt.Sprintf("%.6f,%.6f", stop.Lon, stop.Lat)
	}
	endpoint := fmt.Sprintf("%s/table/v1/driving/%s?annotations=duration",
		strings.TrimRight(c.baseURL(), "/"), strings.Join(coordinates, ";"))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build OSRM request: %w", err)
	}
	request.Header.Set("User-Agent", c.UserAgent)

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("OSRM table request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSRM table request failed: %s", response.Status)
	}

	var payload osrmTableResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OSRM table response: %w", err)
	}
	if payload.Code != "Ok" {
		return nil, fmt.Errorf("OSRM table error code: %s", payload.Code)
	}
	if len(payload.Durations) != len(stops) {
		return nil, fmt.Errorf("OSRM matrix has %d rows, want %d", len(payload.Durations), len(stops))
	}
	for index, row := range payload.Durations {
		if len(row) != len(stops) {
			return nil, fmt.Errorf("OSRM matrix row %d has %d columns, want %d", index, len(row), len(stops))
		}
	}
	return optimizer.Matrix(payload.Durations), nil
}

func (c *OSRM) baseURL() string {
	if c.BaseURL == "" {
		return DefaultOSRMBaseURL
	}
	return c.BaseURL
}

func (c *OSRM) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return c.HTTPClient
}

type osrmTableResponse struct {
	Code      string      `json:"code"`
	Durations [][]float64 `json:"durations"`
}
