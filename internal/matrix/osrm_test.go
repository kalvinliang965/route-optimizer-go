package matrix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"route-optimizer-go/internal/optimizer"
)

func TestDefaultOSRMBaseURLUsesHTTPS(t *testing.T) {
	if !strings.HasPrefix(DefaultOSRMBaseURL, "https://") {
		t.Fatalf("DefaultOSRMBaseURL = %q, want HTTPS", DefaultOSRMBaseURL)
	}
}

func TestOSRMDurations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/table/v1/driving/-73.000000,40.000000;-73.100000,40.100000") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("annotations") != "duration" {
			t.Errorf("annotations = %q", r.URL.Query().Get("annotations"))
		}
		_, _ = w.Write([]byte(`{"code":"Ok","durations":[[0,12],[15,0]]}`))
	}))
	defer server.Close()

	client := NewOSRM(server.URL, "test-agent", 0)
	got, err := client.Durations(context.Background(), []optimizer.Stop{
		{Lat: 40, Lon: -73},
		{Lat: 40.1, Lon: -73.1},
	})
	if err != nil {
		t.Fatalf("Durations: %v", err)
	}
	if got[0][1] != 12 || got[1][0] != 15 {
		t.Fatalf("matrix = %#v", got)
	}
}
