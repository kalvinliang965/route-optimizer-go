package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
)

type mountedOptimizer struct {
	calls int
}

func (f *mountedOptimizer) Optimize(_ context.Context, _ planner.OptimizeRequest) (planner.OptimizeResult, error) {
	f.calls++
	return planner.OptimizeResult{TopK: 1, Routes: []planner.PlannedRoute{{Rank: 1, Path: []int{0, 0}}}}, nil
}

type mountedGeocoder struct {
	calls int
}

func mountedLimits() Limits {
	return Limits{DefaultTopK: 5, MaxTopK: 20, MaxStops: 10}
}

func (f *mountedGeocoder) Geocode(_ context.Context, address string) (optimizer.Stop, error) {
	f.calls++
	return optimizer.Stop{Name: address + " resolved", Lat: 40.75, Lon: -73.98}, nil
}

func TestServerHealthRoute(t *testing.T) {
	server, err := NewServer(&mountedOptimizer{}, &mountedGeocoder{}, mountedLimits())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestServerServesFrontendAtRoot(t *testing.T) {
	server, err := NewServer(&mountedOptimizer{}, &mountedGeocoder{}, mountedLimits())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if !strings.Contains(response.Body.String(), "Route Optimizer") {
		t.Fatalf("body does not contain frontend marker")
	}
}

func TestServerMountsV1OptimizeRoute(t *testing.T) {
	optimizer := &mountedOptimizer{}
	server, err := NewServer(optimizer, &mountedGeocoder{}, mountedLimits())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/optimize", strings.NewReader(`{
		"stops":[
			{"name":"Depot","lat":40.75,"lon":-73.98},
			{"name":"A","lat":40.72,"lon":-74.00}
		],
		"top_k":1
	}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if optimizer.calls != 1 {
		t.Fatalf("optimizer calls = %d, want 1", optimizer.calls)
	}
	var result planner.OptimizeResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].Rank != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestServerMountsV1GeocodeRoute(t *testing.T) {
	geocoder := &mountedGeocoder{}
	server, err := NewServer(&mountedOptimizer{}, geocoder, mountedLimits())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/geocode", strings.NewReader(`{"addresses":["Depot"]}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if geocoder.calls != 1 {
		t.Fatalf("geocoder calls = %d, want 1", geocoder.calls)
	}
	var result struct {
		Results []struct {
			Index int             `json:"index"`
			Stop  *optimizer.Stop `json:"stop"`
		} `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Index != 0 || result.Results[0].Stop == nil || result.Results[0].Stop.ID != "stop-0" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServerMountsV1ConfigRoute(t *testing.T) {
	server, err := NewServer(&mountedOptimizer{}, &mountedGeocoder{}, mountedLimits())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result Limits
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result != mountedLimits() {
		t.Fatalf("limits = %#v, want %#v", result, mountedLimits())
	}
}
