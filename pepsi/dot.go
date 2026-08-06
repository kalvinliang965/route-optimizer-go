package pepsi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultDOTTrafficEndpoint = "https://data.cityofnewyork.us/resource/i4gi-tjb9.json"
	DefaultDOTChunkSize       = 40
	DefaultDOTLimitPerLink    = 3
	DefaultDOTMatchLimit      = 1000

	dotSpeedSelectFields = "link_id,speed,data_as_of"
	dotMatchSelectFields = "link_id,data_as_of,link_points"
)

type DOTClient struct {
	Endpoint     string
	AppToken     string
	UserAgent    string
	HTTPClient   *http.Client
	ChunkSize    int
	LimitPerLink int
}

type DOTTrafficRecord struct {
	LinkID          string `json:"link_id"`
	Speed           string `json:"speed"`
	TravelTime      string `json:"travel_time"`
	Status          string `json:"status"`
	DataAsOf        string `json:"data_as_of"`
	LinkName        string `json:"link_name"`
	Borough         string `json:"borough"`
	LinkPoints      string `json:"link_points"`
	EncodedPolyline string `json:"encoded_poly_line"`
}

type DOTEdgeStateFetcher struct {
	recordsByLinkID     map[string]DOTTrafficRecord
	previousMultipliers map[[2]int]float64
}

func NewDOTClient(endpoint string) DOTClient {
	if endpoint == "" {
		endpoint = DefaultDOTTrafficEndpoint
	}
	return DOTClient{
		Endpoint:     endpoint,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
		UserAgent:    DefaultUserAgent,
		ChunkSize:    DefaultDOTChunkSize,
		LimitPerLink: DefaultDOTLimitPerLink,
	}
}

func BuildDOTEdgeStateFetcher(ctx context.Context, client DOTClient, metadata EdgeMetadataFile, previousMultipliers map[[2]int]float64) (DOTEdgeStateFetcher, error) {
	linkIDs := uniqueMatchedDOTLinkIDs(metadata.Edges)
	if len(linkIDs) == 0 {
		return DOTEdgeStateFetcher{
			recordsByLinkID:     map[string]DOTTrafficRecord{},
			previousMultipliers: copyPreviousMultipliers(previousMultipliers),
		}, nil
	}

	records, err := client.FetchTrafficRecords(ctx, linkIDs)
	if err != nil {
		return DOTEdgeStateFetcher{}, err
	}
	return NewDOTEdgeStateFetcher(records, previousMultipliers), nil
}

func NewDOTEdgeStateFetcher(records []DOTTrafficRecord, previousMultipliers map[[2]int]float64) DOTEdgeStateFetcher {
	return DOTEdgeStateFetcher{
		recordsByLinkID:     latestDOTRecordsByLinkID(records),
		previousMultipliers: copyPreviousMultipliers(previousMultipliers),
	}
}

func (f DOTEdgeStateFetcher) FetchEdgeState(ctx context.Context, edge EdgeMetadata) (EdgeTrafficState, error) {
	if len(edge.MatchedDOTLinkIDs) == 0 {
		return EdgeTrafficState{HasData: false}, nil
	}
	if edge.BaselineDurationSec <= 0 {
		return EdgeTrafficState{}, fmt.Errorf("baseline duration must be > 0, got %v", edge.BaselineDurationSec)
	}
	if edge.BaselineDistanceM <= 0 {
		return EdgeTrafficState{}, fmt.Errorf("baseline distance must be > 0, got %v", edge.BaselineDistanceM)
	}

	var speeds []float64
	for _, linkID := range edge.MatchedDOTLinkIDs {
		record, ok := f.recordsByLinkID[linkID]
		if !ok {
			continue
		}
		speed, err := parsePositiveFloat(record.Speed)
		if err != nil {
			continue
		}
		speeds = append(speeds, speed)
	}
	if len(speeds) == 0 {
		return EdgeTrafficState{HasData: false}, nil
	}

	observedMPH := average(speeds)
	baselineMPH := metersPerSecondToMPH(edge.BaselineDistanceM / edge.BaselineDurationSec)
	currentMultiplier := baselineMPH / observedMPH

	previous, hasPrevious := f.previousMultipliers[[2]int{edge.FromStop, edge.ToStop}]
	return EdgeTrafficState{
		HasData:            true,
		CurrentMultiplier:  currentMultiplier,
		PreviousMultiplier: previous,
		HasPrevious:        hasPrevious,
	}, nil
}

func (c DOTClient) FetchTrafficRecords(ctx context.Context, linkIDs []string) ([]DOTTrafficRecord, error) {
	linkIDs = uniqueSortedStrings(linkIDs)
	if len(linkIDs) == 0 {
		return nil, nil
	}

	chunkSize := c.chunkSize()
	var records []DOTTrafficRecord
	for start := 0; start < len(linkIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(linkIDs) {
			end = len(linkIDs)
		}
		chunkRecords, err := c.fetchTrafficRecordChunk(ctx, linkIDs[start:end])
		if err != nil {
			return nil, err
		}
		records = append(records, chunkRecords...)
	}
	return records, nil
}

func (c DOTClient) fetchTrafficRecordChunk(ctx context.Context, linkIDs []string) ([]DOTTrafficRecord, error) {
	q := url.Values{}
	q.Set("$select", dotSpeedSelectFields)
	q.Set("$where", fmt.Sprintf("link_id in(%s)", soqlStringList(linkIDs)))
	q.Set("$order", "data_as_of DESC")
	q.Set("$limit", strconv.Itoa(len(linkIDs)*c.limitPerLink()))

	return c.fetchTrafficRecordsWithQuery(ctx, q)
}

// FetchRecentTrafficRecords returns one bounded, newest-first window for local
// link-geometry matching. The NYC DOT source is an append-only historical feed,
// so attempting to paginate all rows is neither necessary nor practical.
func (c DOTClient) FetchRecentTrafficRecords(ctx context.Context, limit int) ([]DOTTrafficRecord, error) {
	if limit <= 0 {
		limit = DefaultDOTMatchLimit
	}

	q := url.Values{}
	q.Set("$select", dotMatchSelectFields)
	q.Set("$order", "data_as_of DESC,link_id ASC")
	q.Set("$limit", strconv.Itoa(limit))

	return c.fetchTrafficRecordsWithQuery(ctx, q)
}

func (c DOTClient) fetchTrafficRecordsWithQuery(ctx context.Context, q url.Values) ([]DOTTrafficRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(), nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = q.Encode()
	if c.AppToken != "" {
		req.Header.Set("X-App-Token", c.AppToken)
	}
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
		return nil, fmt.Errorf("dot traffic request failed: %s", resp.Status)
	}

	var records []DOTTrafficRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("parse dot traffic response: %w", err)
	}
	return records, nil
}

func (c DOTClient) endpoint() string {
	if c.Endpoint == "" {
		return DefaultDOTTrafficEndpoint
	}
	return c.Endpoint
}

func (c DOTClient) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return c.HTTPClient
}

func (c DOTClient) userAgent() string {
	if c.UserAgent == "" {
		return DefaultUserAgent
	}
	return c.UserAgent
}

func (c DOTClient) chunkSize() int {
	if c.ChunkSize <= 0 {
		return DefaultDOTChunkSize
	}
	return c.ChunkSize
}

func (c DOTClient) limitPerLink() int {
	if c.LimitPerLink <= 0 {
		return DefaultDOTLimitPerLink
	}
	return c.LimitPerLink
}

func uniqueMatchedDOTLinkIDs(edges []EdgeMetadata) []string {
	var linkIDs []string
	for _, edge := range edges {
		linkIDs = append(linkIDs, edge.MatchedDOTLinkIDs...)
	}
	return uniqueSortedStrings(linkIDs)
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func soqlStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "''")+"'")
	}
	return strings.Join(quoted, ",")
}

func latestDOTRecordsByLinkID(records []DOTTrafficRecord) map[string]DOTTrafficRecord {
	out := make(map[string]DOTTrafficRecord, len(records))
	for _, record := range records {
		linkID := strings.TrimSpace(record.LinkID)
		if linkID == "" {
			continue
		}
		record.LinkID = linkID
		existing, exists := out[linkID]
		if !exists || isNewerDOTRecord(record, existing) {
			out[linkID] = record
		}
	}
	return out
}

func isNewerDOTRecord(candidate, existing DOTTrafficRecord) bool {
	candidateTime, candidateOK := parseDOTTime(candidate.DataAsOf)
	existingTime, existingOK := parseDOTTime(existing.DataAsOf)
	if candidateOK && existingOK {
		return candidateTime.After(existingTime)
	}
	if candidateOK != existingOK {
		return candidateOK
	}
	return false
}

func parseDOTTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parsePositiveFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("value must be > 0, got %v", parsed)
	}
	return parsed, nil
}

func metersPerSecondToMPH(mps float64) float64 {
	return mps * 2.2369362921
}

func average(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func copyPreviousMultipliers(previous map[[2]int]float64) map[[2]int]float64 {
	if len(previous) == 0 {
		return nil
	}
	out := make(map[[2]int]float64, len(previous))
	for key, value := range previous {
		if math.IsNaN(value) || value <= 0 {
			continue
		}
		out[key] = value
	}
	return out
}
