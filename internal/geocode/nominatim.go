// Package geocode contains address-resolution adapters.
package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"route-optimizer-go/internal/optimizer"
)

const DefaultNominatimBaseURL = "https://nominatim.openstreetmap.org"

type Nominatim struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
}

func NewNominatim(baseURL, userAgent string, timeout time.Duration) *Nominatim {
	if baseURL == "" {
		baseURL = DefaultNominatimBaseURL
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Nominatim{
		BaseURL:    baseURL,
		UserAgent:  userAgent,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (c *Nominatim) Geocode(ctx context.Context, address string) (optimizer.Stop, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return optimizer.Stop{}, fmt.Errorf("address is empty")
	}

	endpoint, err := url.Parse(strings.TrimRight(c.baseURL(), "/") + "/search")
	if err != nil {
		return optimizer.Stop{}, fmt.Errorf("build Nominatim URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("q", address)
	query.Set("format", "json")
	query.Set("limit", "1")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return optimizer.Stop{}, fmt.Errorf("build Nominatim request: %w", err)
	}
	request.Header.Set("User-Agent", c.UserAgent)

	response, err := c.httpClient().Do(request)
	if err != nil {
		return optimizer.Stop{}, fmt.Errorf("Nominatim request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return optimizer.Stop{}, fmt.Errorf("Nominatim request failed: %s", response.Status)
	}

	var results []nominatimResult
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		return optimizer.Stop{}, fmt.Errorf("decode Nominatim response: %w", err)
	}
	if len(results) == 0 {
		return optimizer.Stop{}, fmt.Errorf("no geocode result for %q", address)
	}

	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return optimizer.Stop{}, fmt.Errorf("parse latitude %q: %w", results[0].Lat, err)
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return optimizer.Stop{}, fmt.Errorf("parse longitude %q: %w", results[0].Lon, err)
	}
	return optimizer.Stop{Name: results[0].DisplayName, Lat: lat, Lon: lon}, nil
}

func (c *Nominatim) baseURL() string {
	if c.BaseURL == "" {
		return DefaultNominatimBaseURL
	}
	return c.BaseURL
}

func (c *Nominatim) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: 5 * time.Second}
	}
	return c.HTTPClient
}

type nominatimResult struct {
	DisplayName string `json:"display_name"`
	Lon         string `json:"lon"`
	Lat         string `json:"lat"`
}
