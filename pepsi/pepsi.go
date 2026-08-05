// Package pepsi builds OSRM route geometry metadata for route matrix cells.
package pepsi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"route-optimizer-go/internal/route"
)

const (
	DefaultOSRMRouteBaseURL = "http://router.project-osrm.org"
	DefaultUserAgent        = "pepsi-routing-app/1.0"
	EdgeMetadataSource      = "osrm-route"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type StepMetadata struct {
	Name        string  `json:"name"`
	DurationSec float64 `json:"duration_sec"`
	DistanceM   float64 `json:"distance_m"`
}

type EdgeMetadata struct {
	FromStop            int            `json:"from_stop"`
	ToStop              int            `json:"to_stop"`
	BaselineDurationSec float64        `json:"baseline_duration_sec"`
	BaselineDistanceM   float64        `json:"baseline_distance_m"`
	Geometry            []Coordinate   `json:"geometry"`
	Steps               []StepMetadata `json:"steps"`
	MatchedDOTLinkIDs   []string       `json:"matched_dot_link_ids"`
}

type EdgeMetadataFile struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Source      string         `json:"source"`
	Edges       []EdgeMetadata `json:"edges"`
}

func NewClient(baseURL string) Client {
	if baseURL == "" {
		baseURL = DefaultOSRMRouteBaseURL
	}
	return Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		UserAgent:  DefaultUserAgent,
	}
}

func (c Client) BuildEdgeMetadata(ctx context.Context, stops []route.Stop) (EdgeMetadataFile, error) {
	edges := make([]EdgeMetadata, 0, len(stops)*(len(stops)-1))
	for fromIdx := range stops {
		for toIdx := range stops {
			if fromIdx == toIdx {
				continue
			}
			edge, err := c.FetchEdge(ctx, stops, fromIdx, toIdx)
			if err != nil {
				return EdgeMetadataFile{}, fmt.Errorf("fetch edge %d -> %d: %w", fromIdx, toIdx, err)
			}
			edges = append(edges, edge)
		}
	}

	return EdgeMetadataFile{
		GeneratedAt: time.Now().UTC(),
		Source:      EdgeMetadataSource,
		Edges:       edges,
	}, nil
}

func WriteEdgeMetadata(path string, metadata EdgeMetadataFile) error {
	return route.WriteJSON(path, metadata)
}

func ReadEdgeMetadata(path string) (EdgeMetadataFile, error) {
	var metadata EdgeMetadataFile
	if err := route.ReadJSON(path, &metadata); err != nil {
		return EdgeMetadataFile{}, fmt.Errorf("read edge metadata %s: %w", path, err)
	}
	return metadata, nil
}

func (c Client) FetchEdge(ctx context.Context, stops []route.Stop, fromIdx, toIdx int) (EdgeMetadata, error) {
	body, err := c.fetchEdgeBody(ctx, stops, fromIdx, toIdx)
	if err != nil {
		return EdgeMetadata{}, err
	}
	return parseEdge(fromIdx, toIdx, body)
}

func (c Client) fetchEdgeBody(ctx context.Context, stops []route.Stop, fromIdx, toIdx int) ([]byte, error) {
	if fromIdx < 0 || fromIdx >= len(stops) {
		return nil, fmt.Errorf("from stop index out of bounds: %d", fromIdx)
	}
	if toIdx < 0 || toIdx >= len(stops) {
		return nil, fmt.Errorf("to stop index out of bounds: %d", toIdx)
	}
	if fromIdx == toIdx {
		return nil, fmt.Errorf("cannot fetch route geometry for diagonal edge %d -> %d", fromIdx, toIdx)
	}

	from := stops[fromIdx]
	to := stops[toIdx]
	endpoint := fmt.Sprintf("%s/route/v1/driving/%.6f,%.6f;%.6f,%.6f",
		strings.TrimRight(c.baseURL(), "/"),
		from.Lon, from.Lat,
		to.Lon, to.Lat,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("overview", "full")
	q.Set("steps", "true")
	q.Set("geometries", "geojson")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osrm route request failed: %s", resp.Status)
	}

	return body, nil
}

func parseEdge(fromIdx, toIdx int, body []byte) (EdgeMetadata, error) {
	var routeResp osrmRouteResponse
	if err := json.Unmarshal(body, &routeResp); err != nil {
		return EdgeMetadata{}, fmt.Errorf("parse osrm route response: %w", err)
	}
	if routeResp.Code != "Ok" {
		return EdgeMetadata{}, fmt.Errorf("osrm route api error code: %s", routeResp.Code)
	}
	if len(routeResp.Routes) == 0 {
		return EdgeMetadata{}, fmt.Errorf("osrm route response contained no routes")
	}

	best := routeResp.Routes[0]
	geometry, err := convertCoordinates(best.Geometry.Coordinates)
	if err != nil {
		return EdgeMetadata{}, err
	}

	return EdgeMetadata{
		FromStop:            fromIdx,
		ToStop:              toIdx,
		BaselineDurationSec: best.Duration,
		BaselineDistanceM:   best.Distance,
		Geometry:            geometry,
		Steps:               collectSteps(best.Legs),
		MatchedDOTLinkIDs:   []string{},
	}, nil
}

func (c Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultOSRMRouteBaseURL
	}
	return c.BaseURL
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return c.HTTPClient
}

func (c Client) userAgent() string {
	if c.UserAgent == "" {
		return DefaultUserAgent
	}
	return c.UserAgent
}

func convertCoordinates(raw [][]float64) ([]Coordinate, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("osrm route response contained empty geometry")
	}

	coords := make([]Coordinate, 0, len(raw))
	for i, pair := range raw {
		if len(pair) != 2 {
			return nil, fmt.Errorf("osrm geometry coordinate %d has %d values, want 2", i, len(pair))
		}
		coords = append(coords, Coordinate{
			Lat: pair[1],
			Lon: pair[0],
		})
	}
	return coords, nil
}

func collectSteps(legs []osrmLeg) []StepMetadata {
	var steps []StepMetadata
	for _, leg := range legs {
		for _, step := range leg.Steps {
			steps = append(steps, StepMetadata{
				Name:        step.Name,
				DurationSec: step.Duration,
				DistanceM:   step.Distance,
			})
		}
	}
	return steps
}

type osrmRouteResponse struct {
	Code   string      `json:"code"`
	Routes []osrmRoute `json:"routes"`
}

type osrmRoute struct {
	Geometry osrmGeometry `json:"geometry"`
	Duration float64      `json:"duration"`
	Distance float64      `json:"distance"`
	Legs     []osrmLeg    `json:"legs"`
}

type osrmGeometry struct {
	Coordinates [][]float64 `json:"coordinates"`
}

type osrmLeg struct {
	Steps []osrmStep `json:"steps"`
}

type osrmStep struct {
	Name     string  `json:"name"`
	Duration float64 `json:"duration"`
	Distance float64 `json:"distance"`
}
