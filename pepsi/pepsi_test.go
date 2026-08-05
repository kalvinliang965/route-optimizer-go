package pepsi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"route-optimizer-go/internal/route"
)

func TestBuildEdgeMetadataFetchesAllDirectedEdges(t *testing.T) {
	stops := fixtureStops()
	var calls []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/route/v1/driving/-73.974688,40.729661;-73.985972,40.757010":
			w.Write([]byte(osrmRouteJSON(stops[0], stops[1], "forward")))
		case "/route/v1/driving/-73.985972,40.757010;-73.974688,40.729661":
			w.Write([]byte(osrmRouteJSON(stops[1], stops[0], "reverse")))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	artifact, err := client.BuildEdgeMetadata(context.Background(), stops)
	if err != nil {
		t.Fatalf("BuildEdgeMetadata: %v", err)
	}

	if artifact.GeneratedAt.IsZero() {
		t.Fatal("GeneratedAt is zero")
	}
	if artifact.Source != EdgeMetadataSource {
		t.Fatalf("Source = %q, want %q", artifact.Source, EdgeMetadataSource)
	}
	if len(artifact.Edges) != 2 {
		t.Fatalf("edges length = %d, want 2", len(artifact.Edges))
	}
	if len(calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(calls))
	}

	assertEdgeEndpoints(t, artifact.Edges[0], 0, 1, stops[0], stops[1])
	assertEdgeEndpoints(t, artifact.Edges[1], 1, 0, stops[1], stops[0])
}

func TestBuildEdgeMetadataPropagatesEdgeError(t *testing.T) {
	stops := fixtureStops()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 2 {
			http.Error(w, "osrm down", http.StatusBadGateway)
			return
		}
		w.Write([]byte(osrmRouteJSON(stops[0], stops[1], "forward")))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.BuildEdgeMetadata(context.Background(), stops)
	if err == nil {
		t.Fatal("BuildEdgeMetadata error = nil, want fetch error")
	}
	if !strings.Contains(err.Error(), "fetch edge 1 -> 0") {
		t.Fatalf("BuildEdgeMetadata error = %q, want edge context", err)
	}
}

func TestEdgeMetadataRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge_metadata.json")
	want := EdgeMetadataFile{
		GeneratedAt: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
		Source:      EdgeMetadataSource,
		Edges: []EdgeMetadata{
			{
				FromStop:            0,
				ToStop:              1,
				BaselineDurationSec: 42,
				BaselineDistanceM:   1200,
				Geometry: []Coordinate{
					{Lat: 40.729661, Lon: -73.974688},
					{Lat: 40.757010, Lon: -73.985972},
				},
				Steps: []StepMetadata{
					{Name: "1st Avenue", DurationSec: 42, DistanceM: 1200},
				},
				MatchedDOTLinkIDs: []string{},
			},
		},
	}

	if err := WriteEdgeMetadata(path, want); err != nil {
		t.Fatalf("WriteEdgeMetadata: %v", err)
	}
	got, err := ReadEdgeMetadata(path)
	if err != nil {
		t.Fatalf("ReadEdgeMetadata: %v", err)
	}

	if !got.GeneratedAt.Equal(want.GeneratedAt) {
		t.Fatalf("GeneratedAt = %v, want %v", got.GeneratedAt, want.GeneratedAt)
	}
	if got.Source != want.Source {
		t.Fatalf("Source = %q, want %q", got.Source, want.Source)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("edges length = %d, want 1", len(got.Edges))
	}
	assertEdgeEndpoints(t, got.Edges[0], 0, 1,
		route.Stop{Lat: want.Edges[0].Geometry[0].Lat, Lon: want.Edges[0].Geometry[0].Lon},
		route.Stop{Lat: want.Edges[0].Geometry[1].Lat, Lon: want.Edges[0].Geometry[1].Lon},
	)
	if got.Edges[0].Steps[0].Name != "1st Avenue" {
		t.Fatalf("step name = %q, want 1st Avenue", got.Edges[0].Steps[0].Name)
	}
}

func TestParseEdgeFromOSRMFixture(t *testing.T) {
	edge, err := parseEdge(0, 1, readOSRMRouteFixture(t))
	if err != nil {
		t.Fatalf("parseEdge: %v", err)
	}
	assertFixtureEdge(t, edge)
}

func TestFetchEdgeUsesOSRMRouteEndpoint(t *testing.T) {
	stops := fixtureStops()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/route/v1/driving/-73.974688,40.729661;-73.985972,40.757010"
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query().Get("overview"); got != "full" {
			t.Fatalf("overview = %q, want full", got)
		}
		if got := r.URL.Query().Get("steps"); got != "true" {
			t.Fatalf("steps = %q, want true", got)
		}
		if got := r.URL.Query().Get("geometries"); got != "geojson" {
			t.Fatalf("geometries = %q, want geojson", got)
		}
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q, want test-agent", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(readOSRMRouteFixture(t))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.UserAgent = "test-agent"

	edge, err := client.FetchEdge(context.Background(), stops, 0, 1)
	if err != nil {
		t.Fatalf("FetchEdge: %v", err)
	}

	if edge.FromStop != 0 || edge.ToStop != 1 {
		t.Fatalf("edge indices = %d -> %d, want 0 -> 1", edge.FromStop, edge.ToStop)
	}
	assertFixtureEdge(t, edge)
}

func TestFetchEdgeRejectsDiagonalEdge(t *testing.T) {
	stops := fixtureStops()[:1]

	client := NewClient("http://example.invalid")
	_, err := client.FetchEdge(context.Background(), stops, 0, 0)
	if err == nil {
		t.Fatal("FetchEdge error = nil, want diagonal edge error")
	}
	if !strings.Contains(err.Error(), "diagonal edge") {
		t.Fatalf("FetchEdge error = %q, want diagonal edge", err)
	}
}

func TestFetchEdgeReportsNonOKStatus(t *testing.T) {
	stops := fixtureStops()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.FetchEdge(context.Background(), stops, 0, 1)
	if err == nil {
		t.Fatal("FetchEdge error = nil, want HTTP status error")
	}
	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("FetchEdge error = %q, want status", err)
	}
}

func TestRecordOSRMRouteFixture(t *testing.T) {
	if os.Getenv("PEPSI_RECORD_OSRM_FIXTURE") != "1" {
		t.Skip("set PEPSI_RECORD_OSRM_FIXTURE=1 to record pepsi/testdata/osrm_route_response.json")
	}

	client := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	body, err := client.fetchEdgeBody(ctx, fixtureStops(), 0, 1)
	if err != nil {
		t.Fatalf("fetch fixture body: %v", err)
	}
	edge, err := parseEdge(0, 1, body)
	if err != nil {
		t.Fatalf("parse recorded fixture: %v", err)
	}
	assertFixtureEdge(t, edge)

	path := osrmRouteFixturePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("recorded %s (%d bytes)", path, len(body))
}

func TestFetchEdgeIntegrationOSRM(t *testing.T) {
	if os.Getenv("PEPSI_RUN_OSRM_INTEGRATION") != "1" {
		t.Skip("set PEPSI_RUN_OSRM_INTEGRATION=1 to call the public OSRM service")
	}

	client := NewClient("")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	edge, err := client.FetchEdge(ctx, fixtureStops(), 0, 1)
	if err != nil {
		t.Fatalf("FetchEdge integration: %v", err)
	}
	assertFixtureEdge(t, edge)
}

func fixtureStops() []route.Stop {
	return []route.Stop{
		{Name: "Depot", Lat: 40.729661, Lon: -73.974688},
		{Name: "Times Square", Lat: 40.757010, Lon: -73.985972},
	}
}

func osrmRouteFixturePath() string {
	return filepath.Join("testdata", "osrm_route_response.json")
}

func readOSRMRouteFixture(t *testing.T) []byte {
	t.Helper()

	body, err := os.ReadFile(osrmRouteFixturePath())
	if err != nil {
		t.Fatalf("read OSRM fixture: %v; regenerate with PEPSI_RECORD_OSRM_FIXTURE=1 go test ./pepsi -run TestRecordOSRMRouteFixture", err)
	}
	return body
}

func assertFixtureEdge(t *testing.T, edge EdgeMetadata) {
	t.Helper()

	if edge.FromStop != 0 || edge.ToStop != 1 {
		t.Fatalf("edge indices = %d -> %d, want 0 -> 1", edge.FromStop, edge.ToStop)
	}
	if edge.BaselineDurationSec <= 0 {
		t.Fatalf("duration = %v, want > 0", edge.BaselineDurationSec)
	}
	if edge.BaselineDistanceM <= 0 {
		t.Fatalf("distance = %v, want > 0", edge.BaselineDistanceM)
	}
	if len(edge.Geometry) < 2 {
		t.Fatalf("geometry length = %d, want >= 2", len(edge.Geometry))
	}
	if len(edge.Steps) == 0 {
		t.Fatal("steps are empty")
	}
	if edge.MatchedDOTLinkIDs == nil || len(edge.MatchedDOTLinkIDs) != 0 {
		t.Fatalf("matched DOT links = %#v, want empty non-nil slice", edge.MatchedDOTLinkIDs)
	}

	stops := fixtureStops()
	first := edge.Geometry[0]
	last := edge.Geometry[len(edge.Geometry)-1]
	if d := metersBetween(first, stops[0]); d > 500 {
		t.Fatalf("first geometry point is %.1fm from source, want <= 500m", d)
	}
	if d := metersBetween(last, stops[1]); d > 500 {
		t.Fatalf("last geometry point is %.1fm from destination, want <= 500m", d)
	}
}

func metersBetween(c Coordinate, s route.Stop) float64 {
	const earthRadiusM = 6371000.0
	lat1 := c.Lat * math.Pi / 180
	lat2 := s.Lat * math.Pi / 180
	dlat := (s.Lat - c.Lat) * math.Pi / 180
	dlon := (s.Lon - c.Lon) * math.Pi / 180

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func assertEdgeEndpoints(t *testing.T, edge EdgeMetadata, fromIdx, toIdx int, from, to route.Stop) {
	t.Helper()

	if edge.FromStop != fromIdx || edge.ToStop != toIdx {
		t.Fatalf("edge indices = %d -> %d, want %d -> %d", edge.FromStop, edge.ToStop, fromIdx, toIdx)
	}
	if edge.BaselineDurationSec <= 0 {
		t.Fatalf("duration = %v, want > 0", edge.BaselineDurationSec)
	}
	if edge.BaselineDistanceM <= 0 {
		t.Fatalf("distance = %v, want > 0", edge.BaselineDistanceM)
	}
	if len(edge.Geometry) < 2 {
		t.Fatalf("geometry length = %d, want >= 2", len(edge.Geometry))
	}
	if d := metersBetween(edge.Geometry[0], from); d > 1 {
		t.Fatalf("first geometry point is %.1fm from source, want <= 1m", d)
	}
	if d := metersBetween(edge.Geometry[len(edge.Geometry)-1], to); d > 1 {
		t.Fatalf("last geometry point is %.1fm from destination, want <= 1m", d)
	}
	if edge.MatchedDOTLinkIDs == nil || len(edge.MatchedDOTLinkIDs) != 0 {
		t.Fatalf("matched DOT links = %#v, want empty non-nil slice", edge.MatchedDOTLinkIDs)
	}
}

func osrmRouteJSON(from, to route.Stop, stepName string) string {
	return fmt.Sprintf(`{
		"code": "Ok",
		"routes": [{
			"duration": 420.5,
			"distance": 1800.25,
			"geometry": {
				"type": "LineString",
				"coordinates": [
					[%.6f, %.6f],
					[%.6f, %.6f],
					[%.6f, %.6f]
				]
			},
			"legs": [{
				"steps": [
					{"name": %q, "duration": 420.5, "distance": 1800.25}
				]
			}]
		}]
	}`, from.Lon, from.Lat, midpoint(from.Lon, to.Lon), midpoint(from.Lat, to.Lat), to.Lon, to.Lat, stepName)
}

func midpoint(a, b float64) float64 {
	return (a + b) / 2
}
