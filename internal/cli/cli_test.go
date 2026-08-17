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

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
	"route-optimizer-go/internal/storage"
)

func TestRunArgsHelp(t *testing.T) {
	output := captureStdout(t, func() {
		if err := RunArgs(nil); err != nil {
			t.Fatalf("RunArgs: %v", err)
		}
	})
	for _, want := range []string{"route-cli all", "optimize", "-top-k 5"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help missing %q:\n%s", want, output)
		}
	}
}

func TestOptimizeUsesVariableTopK(t *testing.T) {
	directory := t.TempDir()
	configPath := writeTestConfig(t, directory, "", "")
	stopsPath := filepath.Join(directory, "stops.json")
	matrixPath := filepath.Join(directory, "matrix.json")
	planPath := filepath.Join(directory, "optimization.json")
	stops := []optimizer.Stop{{Name: "Depot"}, {Name: "A"}, {Name: "B"}}
	durations := optimizer.Matrix{{0, 1, 5}, {4, 0, 1}, {1, 5, 0}}
	if err := storage.WriteJSON(stopsPath, stops); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteJSON(matrixPath, durations); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		err := RunArgs([]string{"optimize", "-config", configPath, "-stops", stopsPath, "-matrix", matrixPath, "-out", planPath, "-top-k", "2"})
		if err != nil {
			t.Fatalf("RunArgs: %v", err)
		}
	})
	if !strings.Contains(output, "Top 2 route(s)") || !strings.Contains(output, "#1  0.05 mins") {
		t.Fatalf("output:\n%s", output)
	}
	if strings.Contains(output, "https://www.google.com/maps/dir/") {
		t.Fatalf("optimize should not build Maps links:\n%s", output)
	}
	var result planner.OptimizeResult
	if err := storage.ReadJSON(planPath, &result); err != nil {
		t.Fatalf("read optimization result: %v", err)
	}
	if len(result.Routes) != 2 || result.Routes[0].DirectionsURL != "" {
		t.Fatalf("optimization result = %#v", result)
	}
}

func TestItineraryReadsPlanAndBuildsSelectedMapsRoute(t *testing.T) {
	directory := t.TempDir()
	configPath := writeTestConfig(t, directory, "", "")
	planPath := filepath.Join(directory, "optimization.json")
	result := planner.OptimizeResult{
		Stops: []optimizer.Stop{{Name: "Depot", Lat: 40, Lon: -73}, {Name: "A", Lat: 41, Lon: -74}},
		TopK:  2,
		Routes: []planner.PlannedRoute{
			{Rank: 1, Path: []int{0, 1, 0}, OrderedStops: []optimizer.Stop{{Name: "Depot"}, {Name: "A"}, {Name: "Depot"}}, DurationSeconds: 120},
			{Rank: 2, Path: []int{0, 1, 0}, OrderedStops: []optimizer.Stop{{Name: "Depot"}, {Name: "A"}, {Name: "Depot"}}, DurationSeconds: 180},
		},
	}
	if err := storage.WriteJSON(planPath, result); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := RunArgs([]string{"itinerary", "-config", configPath, "-plan", planPath, "-rank", "2"}); err != nil {
			t.Fatalf("RunArgs: %v", err)
		}
	})
	if strings.Contains(output, "#1") || !strings.Contains(output, "#2  3.00 mins") {
		t.Fatalf("itinerary selected wrong rank:\n%s", output)
	}
	if strings.Count(output, "https://www.google.com/maps/dir/") != 1 {
		t.Fatalf("expected one Maps link:\n%s", output)
	}
}

func TestAllRunsThroughPlannerAndAdapters(t *testing.T) {
	directory := t.TempDir()
	addressesPath := filepath.Join(directory, "addresses.txt")
	if err := os.WriteFile(addressesPath, []byte("Depot\nStop A\n"), 0644); err != nil {
		t.Fatal(err)
	}

	geocodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("q")
		lat, lon := "40.000000", "-73.000000"
		if name == "Stop A" {
			lat, lon = "40.100000", "-73.100000"
		}
		_, _ = fmt.Fprintf(w, `[{"display_name":%q,"lat":%q,"lon":%q}]`, name, lat, lon)
	}))
	defer geocodeServer.Close()

	matrixServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"Ok","durations":[[0,60],[90,0]]}`))
	}))
	defer matrixServer.Close()

	configPath := writeTestConfig(t, directory, geocodeServer.URL, matrixServer.URL)
	stopsPath := filepath.Join(directory, "artifacts", "stops.json")
	matrixPath := filepath.Join(directory, "artifacts", "matrix.json")
	planPath := filepath.Join(directory, "artifacts", "optimization.json")
	output := captureStdout(t, func() {
		err := RunArgs([]string{
			"all", "-config", configPath, "-top-k", "1",
			"-stops-out", stopsPath, "-matrix-out", matrixPath, "-plan-out", planPath,
			addressesPath,
		})
		if err != nil {
			t.Fatalf("RunArgs: %v", err)
		}
	})
	for _, want := range []string{"addresses → geocode → matrix", "Top 1 route(s)", "2.50 mins", "all: wrote"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	var stops []optimizer.Stop
	if err := storage.ReadJSON(stopsPath, &stops); err != nil || len(stops) != 2 {
		t.Fatalf("stops artifact = %#v, err %v", stops, err)
	}
	var result planner.OptimizeResult
	if err := storage.ReadJSON(planPath, &result); err != nil || len(result.Routes) != 1 {
		t.Fatalf("optimization artifact = %#v, err %v", result, err)
	}
}

func TestGeocodeReusesPersistentCacheAcrossRuns(t *testing.T) {
	directory := t.TempDir()
	addressesPath := filepath.Join(directory, "addresses.txt")
	if err := os.WriteFile(addressesPath, []byte("Times Square\n"), 0644); err != nil {
		t.Fatal(err)
	}

	requests := 0
	geocodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`[{"display_name":"Times Square, New York","lat":"40.757","lon":"-73.986"}]`))
	}))
	defer geocodeServer.Close()

	configPath := filepath.Join(directory, "config.yaml")
	configBody := fmt.Sprintf(`
planner:
  default_top_k: 5
  max_top_k: 10
  max_stops: 10
providers:
  nominatim_base_url: %q
  osrm_base_url: "http://unused-osrm"
cache:
  enabled: true
  directory: %q
  geocode_ttl_hours: 2160
  matrix_ttl_hours: 720
http:
  geocode_timeout_sec: 2
  matrix_timeout_sec: 2
  user_agent: "cli-test"
output:
  duration_unit: minutes
`, geocodeServer.URL, filepath.Join(directory, "cache"))
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 2; run++ {
		outPath := filepath.Join(directory, fmt.Sprintf("stops-%d.json", run))
		captureStdout(t, func() {
			if err := RunArgs([]string{"geocode", "-config", configPath, "-addresses", addressesPath, "-out", outPath}); err != nil {
				t.Fatalf("RunArgs: %v", err)
			}
		})
	}
	if requests != 1 {
		t.Fatalf("Nominatim requests = %d, want 1", requests)
	}
}

func TestRunArgsRejectsUnknownCommand(t *testing.T) {
	err := RunArgs([]string{"traffic"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v", err)
	}
}

func writeTestConfig(t *testing.T, directory, nominatimURL, osrmURL string) string {
	t.Helper()
	if nominatimURL == "" {
		nominatimURL = "http://unused-nominatim"
	}
	if osrmURL == "" {
		osrmURL = "http://unused-osrm"
	}
	path := filepath.Join(directory, "config.yaml")
	content := fmt.Sprintf(`
planner:
  default_top_k: 5
  max_top_k: 10
  max_stops: 10
providers:
  nominatim_base_url: %q
  osrm_base_url: %q
cache:
  enabled: false
http:
  geocode_timeout_sec: 2
  matrix_timeout_sec: 2
  user_agent: "cli-test"
output:
  duration_unit: minutes
`, nominatimURL, osrmURL)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureStdout(t *testing.T, function func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	function()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}
