package route

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchGeocodeAddress_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Rate limited"))
	}))
	defer srv.Close()

	old := NominatimBaseURL
	NominatimBaseURL = srv.URL
	defer func() { NominatimBaseURL = old }()

	_, err := FetchGeocodeAddress("Times Square, New York, NY")
	if err == nil {
		t.Fatal("expected error for HTTP 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to mention status 429, got: %v", err)
	}
}

func TestFetchDurationMatrix_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream error"))
	}))
	defer srv.Close()

	old := OSRMTableBaseURL
	OSRMTableBaseURL = srv.URL
	defer func() { OSRMTableBaseURL = old }()

	stops := []Stop{
		{Name: "A", Lat: 40.71, Lon: -74.00},
		{Name: "B", Lat: 40.72, Lon: -74.01},
	}
	_, err := FetchDurationMatrix(stops)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}
