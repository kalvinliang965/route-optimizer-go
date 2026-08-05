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
