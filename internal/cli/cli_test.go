package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"route-optimizer-go/internal/route"
	"route-optimizer-go/pepsi"
)

func TestRunItineraryAppliesEdgeStateFixture(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	stopsPath := filepath.Join(tempDir, "stops.json")
	matrixPath := filepath.Join(tempDir, "matrix.json")
	edgeMetadataPath := filepath.Join(tempDir, "edge_metadata.json")
	edgeStateFixturePath := filepath.Join(tempDir, "edge_state_fixture.json")

	if err := os.WriteFile(configPath, []byte(itineraryTrafficTestConfig), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stops := []route.Stop{
		{Name: "Depot", Lat: 40.0, Lon: -73.0},
		{Name: "Stop A", Lat: 40.1, Lon: -73.1},
		{Name: "Stop B", Lat: 40.2, Lon: -73.2},
	}
	if err := route.WriteJSON(stopsPath, stops); err != nil {
		t.Fatalf("write stops: %v", err)
	}

	matrix := [][]float64{
		{0, 10, 5},
		{10, 0, 10},
		{5, 5, 0},
	}
	if err := route.WriteJSON(matrixPath, matrix); err != nil {
		t.Fatalf("write matrix: %v", err)
	}

	metadata := pepsi.EdgeMetadataFile{
		Source: pepsi.EdgeMetadataSource,
		Edges: []pepsi.EdgeMetadata{
			{FromStop: 0, ToStop: 2},
			{FromStop: 2, ToStop: 1},
		},
	}
	if err := pepsi.WriteEdgeMetadata(edgeMetadataPath, metadata); err != nil {
		t.Fatalf("write edge metadata: %v", err)
	}

	fixtureJSON := `{
		"edges": [
			{"from_stop": 0, "to_stop": 2, "current_multiplier": 10.0},
			{"from_stop": 2, "to_stop": 1, "current_multiplier": 10.0}
		]
	}`
	if err := os.WriteFile(edgeStateFixturePath, []byte(fixtureJSON), 0644); err != nil {
		t.Fatalf("write edge state fixture: %v", err)
	}

	output := captureStdout(t, func() {
		err := Run("itinerary", []string{
			"-config", configPath,
			"-stops", stopsPath,
			"-matrix", matrixPath,
			"-edge-metadata", edgeMetadataPath,
			"-edge-state-fixture", edgeStateFixturePath,
			"-traffic-ema-alpha", "1",
			"-traffic-max-multiplier", "10",
		})
		if err != nil {
			t.Fatalf("Run itinerary: %v", err)
		}
	})

	if !strings.Contains(output, "applied traffic fixture") {
		t.Fatalf("output missing traffic fixture banner:\n%s", output)
	}
	if !strings.Contains(output, "#1  0.33 mins") {
		t.Fatalf("output missing adjusted best duration:\n%s", output)
	}
	stopA := strings.Index(output, "Stop A")
	stopB := strings.Index(output, "Stop B")
	if stopA == -1 || stopB == -1 || stopA > stopB {
		t.Fatalf("output route order should visit Stop A before Stop B:\n%s", output)
	}
}

func TestRunItineraryAppliesDOTTraffic(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	stopsPath := filepath.Join(tempDir, "stops.json")
	matrixPath := filepath.Join(tempDir, "matrix.json")
	edgeMetadataPath := filepath.Join(tempDir, "edge_metadata.json")

	if err := os.WriteFile(configPath, []byte(itineraryTrafficTestConfig), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stops := []route.Stop{
		{Name: "Depot", Lat: 40.0, Lon: -73.0},
		{Name: "Stop A", Lat: 40.1, Lon: -73.1},
		{Name: "Stop B", Lat: 40.2, Lon: -73.2},
	}
	if err := route.WriteJSON(stopsPath, stops); err != nil {
		t.Fatalf("write stops: %v", err)
	}

	matrix := [][]float64{
		{0, 10, 5},
		{10, 0, 10},
		{5, 5, 0},
	}
	if err := route.WriteJSON(matrixPath, matrix); err != nil {
		t.Fatalf("write matrix: %v", err)
	}

	metadata := pepsi.EdgeMetadataFile{
		Source: pepsi.EdgeMetadataSource,
		Edges: []pepsi.EdgeMetadata{
			{
				FromStop:            0,
				ToStop:              2,
				BaselineDurationSec: 60,
				BaselineDistanceM:   1609.344,
				MatchedDOTLinkIDs:   []string{"dot-slow-1"},
			},
			{
				FromStop:            2,
				ToStop:              1,
				BaselineDurationSec: 60,
				BaselineDistanceM:   1609.344,
				MatchedDOTLinkIDs:   []string{"dot-slow-2"},
			},
		},
	}
	if err := pepsi.WriteEdgeMetadata(edgeMetadataPath, metadata); err != nil {
		t.Fatalf("write edge metadata: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("X-App-Token"); got != "test-token" {
			t.Fatalf("X-App-Token = %q, want test-token", got)
		}
		if got := r.Header.Get("User-Agent"); got != "cli-test-agent" {
			t.Fatalf("User-Agent = %q, want cli-test-agent", got)
		}
		if got := r.URL.Query().Get("$where"); got != "link_id in('dot-slow-1','dot-slow-2')" {
			t.Fatalf("$where = %q", got)
		}
		if got := r.URL.Query().Get("$limit"); got != "2" {
			t.Fatalf("$limit = %q, want 2", got)
		}
		w.Write([]byte(`[
			{"link_id":"dot-slow-1","speed":"6","data_as_of":"2026-08-05T12:00:00"},
			{"link_id":"dot-slow-2","speed":"6","data_as_of":"2026-08-05T12:00:00"}
		]`))
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		err := Run("itinerary", []string{
			"-config", configPath,
			"-stops", stopsPath,
			"-matrix", matrixPath,
			"-edge-metadata", edgeMetadataPath,
			"-dot-traffic",
			"-dot-endpoint", server.URL,
			"-dot-app-token", "test-token",
			"-dot-limit-per-link", "1",
			"-traffic-ema-alpha", "1",
			"-traffic-max-multiplier", "10",
		})
		if err != nil {
			t.Fatalf("Run itinerary: %v", err)
		}
	})

	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
	if !strings.Contains(output, "applied DOT traffic") {
		t.Fatalf("output missing DOT traffic banner:\n%s", output)
	}
	if !strings.Contains(output, "#1  0.33 mins") {
		t.Fatalf("output missing adjusted best duration:\n%s", output)
	}
	stopA := strings.Index(output, "Stop A")
	stopB := strings.Index(output, "Stop B")
	if stopA == -1 || stopB == -1 || stopA > stopB {
		t.Fatalf("output route order should visit Stop A before Stop B:\n%s", output)
	}
}

func TestRunEdgeMetadataWritesArtifact(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	stopsPath := filepath.Join(tempDir, "stops.json")
	outPath := filepath.Join(tempDir, "edge_metadata.json")

	if err := os.WriteFile(configPath, []byte(edgeMetadataTestConfig), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stops := []route.Stop{
		{Name: "Depot", Lat: 40.729661, Lon: -73.974688},
		{Name: "Times Square", Lat: 40.757010, Lon: -73.985972},
	}
	if err := route.WriteJSON(stopsPath, stops); err != nil {
		t.Fatalf("write stops: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch r.URL.Path {
		case "/route/v1/driving/-73.974688,40.729661;-73.985972,40.757010":
			w.Write([]byte(cliOSRMRouteJSON(stops[0], stops[1], "forward")))
		case "/route/v1/driving/-73.985972,40.757010;-73.974688,40.729661":
			w.Write([]byte(cliOSRMRouteJSON(stops[1], stops[0], "reverse")))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := Run("edge-metadata", []string{
		"-config", configPath,
		"-stops", stopsPath,
		"-out", outPath,
		"-osrm-base-url", server.URL,
	})
	if err != nil {
		t.Fatalf("Run edge-metadata: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}

	artifact, err := pepsi.ReadEdgeMetadata(outPath)
	if err != nil {
		t.Fatalf("ReadEdgeMetadata: %v", err)
	}
	if artifact.Source != pepsi.EdgeMetadataSource {
		t.Fatalf("source = %q, want %q", artifact.Source, pepsi.EdgeMetadataSource)
	}
	if len(artifact.Edges) != 2 {
		t.Fatalf("edges length = %d, want 2", len(artifact.Edges))
	}
	if artifact.Edges[0].FromStop != 0 || artifact.Edges[0].ToStop != 1 {
		t.Fatalf("first edge = %d -> %d, want 0 -> 1", artifact.Edges[0].FromStop, artifact.Edges[0].ToStop)
	}
	if artifact.Edges[1].FromStop != 1 || artifact.Edges[1].ToStop != 0 {
		t.Fatalf("second edge = %d -> %d, want 1 -> 0", artifact.Edges[1].FromStop, artifact.Edges[1].ToStop)
	}
}

func TestRunMatchEdgesWritesMatchedMetadata(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	edgeMetadataPath := filepath.Join(tempDir, "edge_metadata.json")
	dotFixturePath := filepath.Join(tempDir, "dot_records.json")
	outPath := filepath.Join(tempDir, "edge_metadata_matched.json")

	if err := os.WriteFile(configPath, []byte(edgeMetadataTestConfig), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	metadata := pepsi.EdgeMetadataFile{
		Source: pepsi.EdgeMetadataSource,
		Edges: []pepsi.EdgeMetadata{
			{
				FromStop: 0,
				ToStop:   1,
				Geometry: []pepsi.Coordinate{
					{Lat: 40.0000, Lon: -73.0000},
					{Lat: 40.0010, Lon: -73.0010},
				},
			},
		},
	}
	if err := pepsi.WriteEdgeMetadata(edgeMetadataPath, metadata); err != nil {
		t.Fatalf("write edge metadata: %v", err)
	}

	records := []pepsi.DOTTrafficRecord{
		{
			LinkID:     "dot-near",
			LinkPoints: "40.000100,-73.000100 40.000900,-73.000900",
			DataAsOf:   "2026-08-05T12:00:00",
		},
		{
			LinkID:     "dot-far",
			LinkPoints: "40.010000,-73.010000 40.011000,-73.011000",
			DataAsOf:   "2026-08-05T12:00:00",
		},
	}
	if err := route.WriteJSON(dotFixturePath, records); err != nil {
		t.Fatalf("write DOT fixture: %v", err)
	}

	output := captureStdout(t, func() {
		err := Run("match-edges", []string{
			"-config", configPath,
			"-edge-metadata", edgeMetadataPath,
			"-dot-fixture", dotFixturePath,
			"-out", outPath,
			"-match-max-distance-m", "25",
			"-match-max-average-distance-m", "15",
		})
		if err != nil {
			t.Fatalf("Run match-edges: %v", err)
		}
	})

	if !strings.Contains(output, "matched 1/1 edges") {
		t.Fatalf("output missing match summary:\n%s", output)
	}

	matched, err := pepsi.ReadEdgeMetadata(outPath)
	if err != nil {
		t.Fatalf("ReadEdgeMetadata: %v", err)
	}
	got := matched.Edges[0].MatchedDOTLinkIDs
	if len(got) != 1 || got[0] != "dot-near" {
		t.Fatalf("matched IDs = %#v, want dot-near", got)
	}
}

const edgeMetadataTestConfig = `
solver:
  top_k: 5
  max_stops: 15
http:
  geocode_timeout_sec: 5
  osrm_timeout_sec: 10
  user_agent: "cli-test-agent"
output:
  duration_unit: minutes
`

const itineraryTrafficTestConfig = `
solver:
  top_k: 1
  max_stops: 15
http:
  geocode_timeout_sec: 5
  osrm_timeout_sec: 10
  user_agent: "cli-test-agent"
output:
  duration_unit: minutes
`

func cliOSRMRouteJSON(from, to route.Stop, stepName string) string {
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
	}`, from.Lon, from.Lat, (from.Lon+to.Lon)/2, (from.Lat+to.Lat)/2, to.Lon, to.Lat, stepName)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(output)
}
