package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
)

type fakeRouteOptimizer struct {
	result  planner.OptimizeResult
	err     error
	request planner.OptimizeRequest
	calls   int
}

type countingMatrixProvider struct {
	calls int
}

func (p *countingMatrixProvider) Durations(_ context.Context, stops []optimizer.Stop) (optimizer.Matrix, error) {
	p.calls++
	matrix := make(optimizer.Matrix, len(stops))
	for index := range matrix {
		matrix[index] = make([]float64, len(stops))
	}
	return matrix, nil
}

func (f *fakeRouteOptimizer) Optimize(_ context.Context, request planner.OptimizeRequest) (planner.OptimizeResult, error) {
	f.calls++
	f.request = request
	return f.result, f.err
}

func TestOptimizeSuccess(t *testing.T) {
	fake := &fakeRouteOptimizer{
		result: planner.OptimizeResult{
			Stops: []optimizer.Stop{{ID: "depot", Name: "Depot"}, {ID: "a", Name: "Stop A"}},
			TopK:  1,
			Routes: []planner.PlannedRoute{{
				Rank:            1,
				Path:            []int{0, 1, 0},
				DurationSeconds: 660,
				DirectionsURL:   "https://www.google.com/maps/dir/?api=1",
			}},
		},
	}
	server, err := NewServer(fake, &fakeAddressGeocoder{}, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	body := `{
		"stops":[
			{"id":"depot","name":"Depot","lat":40.75,"lon":-73.98},
			{"id":"a","name":"Stop A","lat":40.72,"lon":-74.00}
		],
		"top_k":1
	}`
	request := httptest.NewRequest(http.MethodPost, "/optimize", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if fake.calls != 1 || fake.request.TopK != 1 {
		t.Fatalf("optimizer calls = %d, request = %#v", fake.calls, fake.request)
	}
	if len(fake.request.Stops) != 2 || len(fake.request.DurationMatrixSeconds) != 0 {
		t.Fatalf("optimizer request = %#v", fake.request)
	}

	var result planner.OptimizeResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].DurationSeconds != 660 || result.Routes[0].DirectionsURL == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOptimizeRejectsWrongMethod(t *testing.T) {
	fake := &fakeRouteOptimizer{}
	server, err := NewServer(fake, &fakeAddressGeocoder{}, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/optimize", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", got)
	}
	if fake.calls != 0 {
		t.Fatalf("optimizer calls = %d, want 0", fake.calls)
	}
}

func TestOptimizeRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "syntax", body: `{"stops":`},
		{name: "unknown field", body: `{"unknown":true}`},
		{name: "client matrix is not accepted", body: `{"duration_matrix_seconds":[[0]]}`},
		{name: "multiple values", body: `{}` + `{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeRouteOptimizer{}
			server, err := NewServer(fake, &fakeAddressGeocoder{}, testLimits(10))
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			request := httptest.NewRequest(http.MethodPost, "/optimize", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if fake.calls != 0 {
				t.Fatalf("optimizer calls = %d, want 0", fake.calls)
			}
		})
	}
}

func TestOptimizeRejectsInvalidInputBeforeMatrixProvider(t *testing.T) {
	limits := Limits{DefaultTopK: 2, MaxTopK: 3, MaxStops: 2}
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "missing stops", body: `{}`, wantErr: "at least two stops"},
		{name: "only depot", body: `{"stops":[{"name":"Depot","lat":40,"lon":-73}]}`, wantErr: "at least two stops"},
		{name: "too many stops", body: `{
			"stops":[
				{"name":"Depot","lat":40,"lon":-73},
				{"name":"A","lat":41,"lon":-74},
				{"name":"B","lat":42,"lon":-75}
			]
		}`, wantErr: "too many stops"},
		{name: "blank name", body: `{"stops":[{"name":"   ","lat":40,"lon":-73},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "name is required"},
		{name: "missing latitude", body: `{"stops":[{"name":"Depot","lon":-73},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "latitude is required"},
		{name: "null longitude", body: `{"stops":[{"name":"Depot","lat":40,"lon":null},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "longitude is required"},
		{name: "latitude out of range", body: `{"stops":[{"name":"Depot","lat":91,"lon":-73},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "latitude must be finite"},
		{name: "longitude out of range", body: `{"stops":[{"name":"Depot","lat":40,"lon":-181},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "longitude must be finite"},
		{name: "blank id", body: `{"stops":[{"id":"   ","name":"Depot","lat":40,"lon":-73},{"name":"A","lat":41,"lon":-74}]}`, wantErr: "id cannot be blank"},
		{name: "duplicate id", body: `{
			"stops":[
				{"id":"same","name":"Depot","lat":40,"lon":-73},
				{"id":" same ","name":"A","lat":41,"lon":-74}
			]
		}`, wantErr: "duplicates stop 0"},
		{name: "negative top k", body: `{"stops":[{"name":"Depot","lat":40,"lon":-73},{"name":"A","lat":41,"lon":-74}],"top_k":-1}`, wantErr: "top_k must be >= 1"},
		{name: "top k above limit", body: `{"stops":[{"name":"Depot","lat":40,"lon":-73},{"name":"A","lat":41,"lon":-74}],"top_k":4}`, wantErr: "top_k must be <= 3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &countingMatrixProvider{}
			service := planner.Service{
				Solver:         optimizer.NewSolver(limits.MaxStops, limits.MaxTopK),
				MatrixProvider: provider,
				DefaultTopK:    limits.DefaultTopK,
			}
			server, err := NewServer(service, &fakeAddressGeocoder{}, limits)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}

			request := httptest.NewRequest(http.MethodPost, "/optimize", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
			}
			var result errorResponse
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if !strings.Contains(result.Error, test.wantErr) {
				t.Fatalf("error = %q, want error containing %q", result.Error, test.wantErr)
			}
			if provider.calls != 0 {
				t.Fatalf("matrix provider calls = %d, want 0", provider.calls)
			}
		})
	}
}

func TestOptimizeMapsPlannerErrorToUnprocessableEntity(t *testing.T) {
	fake := &fakeRouteOptimizer{err: errors.New("matrix must be square")}
	server, err := NewServer(fake, &fakeAddressGeocoder{}, testLimits(10))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/optimize", strings.NewReader(`{
		"stops":[
			{"name":"Depot","lat":40,"lon":-73},
			{"name":"A","lat":41,"lon":-74}
		],
		"top_k":1
	}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(response.Body.String(), "matrix must be square") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestNewServerRequiresOptimizer(t *testing.T) {
	if _, err := NewServer(nil, &fakeAddressGeocoder{}, testLimits(10)); err == nil {
		t.Fatal("NewServer nil optimizer error = nil")
	}
	if _, err := NewServer(&fakeRouteOptimizer{}, nil, testLimits(10)); err == nil {
		t.Fatal("NewServer nil geocoder error = nil")
	}
	if _, err := NewServer(&fakeRouteOptimizer{}, &fakeAddressGeocoder{}, Limits{}); err == nil {
		t.Fatal("NewServer invalid max addresses error = nil")
	}
}
