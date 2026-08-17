package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigReturnsPlannerLimits(t *testing.T) {
	want := Limits{DefaultTopK: 2, MaxTopK: 7, MaxStops: 4}
	server, err := NewServer(&fakeRouteOptimizer{}, &fakeAddressGeocoder{}, want)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/config", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var got Limits
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got != want {
		t.Fatalf("limits = %#v, want %#v", got, want)
	}
}

func TestConfigRejectsWrongMethod(t *testing.T) {
	server, err := NewServer(&fakeRouteOptimizer{}, &fakeAddressGeocoder{}, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/config", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
}

func TestNewServerRejectsInvalidLimits(t *testing.T) {
	tests := []Limits{
		{DefaultTopK: 0, MaxTopK: 3, MaxStops: 2},
		{DefaultTopK: 4, MaxTopK: 3, MaxStops: 2},
		{DefaultTopK: 2, MaxTopK: 3, MaxStops: 0},
		{DefaultTopK: 2, MaxTopK: 3, MaxStops: 1},
	}
	for _, limits := range tests {
		if _, err := NewServer(&fakeRouteOptimizer{}, &fakeAddressGeocoder{}, limits); err == nil {
			t.Fatalf("NewServer(%#v) error = nil", limits)
		}
	}
}
