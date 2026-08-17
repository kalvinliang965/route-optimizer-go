package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"route-optimizer-go/internal/optimizer"
)

func TestGeocodeReturnsOrderedPerRowResults(t *testing.T) {
	geocoder := &fakeAddressGeocoder{
		stops: map[string]optimizer.Stop{
			"Times Square": {Name: "Times Square, New York", Lat: 40.757, Lon: -73.986},
			"City Hall":    {ID: "provider-id", Name: "New York City Hall", Lat: 40.713, Lon: -74.006},
		},
		errors: map[string]error{"Unknown": errors.New("no geocode result")},
	}
	server, err := NewServer(&fakeRouteOptimizer{}, geocoder, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/geocode", strings.NewReader(`{
		"addresses":[" Times Square ","Unknown","City Hall"]
	}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !reflect.DeepEqual(geocoder.calls, []string{"Times Square", "Unknown", "City Hall"}) {
		t.Fatalf("geocoder calls = %#v", geocoder.calls)
	}

	var result geocodeResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Results) != 3 {
		t.Fatalf("results = %#v", result.Results)
	}
	if first := result.Results[0]; first.Index != 0 || first.Address != "Times Square" || first.Stop == nil || first.Stop.ID != "stop-0" || first.Error != "" {
		t.Fatalf("first result = %#v", first)
	}
	if second := result.Results[1]; second.Index != 1 || second.Address != "Unknown" || second.Stop != nil || second.Error != "no geocode result" {
		t.Fatalf("second result = %#v", second)
	}
	if third := result.Results[2]; third.Index != 2 || third.Stop == nil || third.Stop.ID != "stop-2" || third.Error != "" {
		t.Fatalf("third result = %#v", third)
	}
}

func TestGeocodeReportsBlankRowWithoutCallingProvider(t *testing.T) {
	geocoder := &fakeAddressGeocoder{}
	server, err := NewServer(&fakeRouteOptimizer{}, geocoder, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/geocode", strings.NewReader(`{"addresses":["   ","Valid"]}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(geocoder.calls, []string{"Valid"}) {
		t.Fatalf("geocoder calls = %#v", geocoder.calls)
	}
	var result geocodeResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Results[0].Address != "" || result.Results[0].Error != "address is empty" || result.Results[0].Stop != nil {
		t.Fatalf("blank result = %#v", result.Results[0])
	}
	if result.Results[1].Stop == nil || result.Results[1].Stop.ID != "stop-1" {
		t.Fatalf("valid result = %#v", result.Results[1])
	}
}

func TestGeocodeRejectsWrongMethod(t *testing.T) {
	geocoder := &fakeAddressGeocoder{}
	server, err := NewServer(&fakeRouteOptimizer{}, geocoder, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/geocode", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", got)
	}
	if len(geocoder.calls) != 0 {
		t.Fatalf("geocoder calls = %#v", geocoder.calls)
	}
}

func TestGeocodeRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "syntax", body: `{"addresses":`},
		{name: "unknown field", body: `{"addresses":["A"],"unknown":true}`},
		{name: "multiple values", body: `{"addresses":["A"]}{}`},
		{name: "too large", body: `{"addresses":["` + strings.Repeat("a", maxRequestBytes) + `"]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			geocoder := &fakeAddressGeocoder{}
			server, err := NewServer(&fakeRouteOptimizer{}, geocoder, testLimits(10))
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			request := httptest.NewRequest(http.MethodPost, "/geocode", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if len(geocoder.calls) != 0 {
				t.Fatalf("geocoder calls = %#v", geocoder.calls)
			}
		})
	}
}

func TestGeocodeRejectsInvalidBatch(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing addresses", body: `{}`},
		{name: "null addresses", body: `{"addresses":null}`},
		{name: "empty addresses", body: `{"addresses":[]}`},
		{name: "too many addresses", body: `{"addresses":["A","B","C"]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			geocoder := &fakeAddressGeocoder{}
			server, err := NewServer(&fakeRouteOptimizer{}, geocoder, testLimits(2))
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			request := httptest.NewRequest(http.MethodPost, "/geocode", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
			}
			if len(geocoder.calls) != 0 {
				t.Fatalf("geocoder calls = %#v", geocoder.calls)
			}
		})
	}
}
