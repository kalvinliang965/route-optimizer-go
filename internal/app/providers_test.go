package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"route-optimizer-go/internal/config"
	"route-optimizer-go/internal/matrix"
	"route-optimizer-go/internal/optimizer"
)

func TestNewMatrixProviderLimitsPublicOSRMInsideCache(t *testing.T) {
	cfg := config.Default()
	cfg.Cache.Directory = t.TempDir()

	provider := NewMatrixProvider(cfg, "https://router.project-osrm.org", nil)
	cached, ok := provider.(*matrix.Cached)
	if !ok {
		t.Fatalf("provider type = %T, want *matrix.Cached", provider)
	}
	if _, ok := cached.Next.(*matrix.RateLimited); !ok {
		t.Fatalf("cached provider type = %T, want *matrix.RateLimited", cached.Next)
	}
}

func TestNewMatrixProviderSharesPersistentCacheAcrossInstances(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"code":"Ok","durations":[[0,12],[15,0]]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Cache.Directory = t.TempDir()
	stops := []optimizer.Stop{{Lat: 40, Lon: -73}, {Lat: 40.1, Lon: -73.1}}

	firstProcess := NewMatrixProvider(cfg, server.URL, nil)
	if _, err := firstProcess.Durations(context.Background(), stops); err != nil {
		t.Fatalf("first Durations: %v", err)
	}

	secondProcess := NewMatrixProvider(cfg, server.URL, nil)
	got, err := secondProcess.Durations(context.Background(), stops)
	if err != nil {
		t.Fatalf("second Durations: %v", err)
	}
	if got[0][1] != 12 || got[1][0] != 15 {
		t.Fatalf("matrix = %#v", got)
	}
	if requests != 1 {
		t.Fatalf("OSRM requests = %d, want 1", requests)
	}
}

func TestNewGeocoderSharesPersistentCacheAcrossInstances(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`[{"display_name":"Times Square, New York","lat":"40.757","lon":"-73.986"}]`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Cache.Directory = t.TempDir()

	firstProcess := NewGeocoder(cfg, server.URL, nil)
	if _, err := firstProcess.Geocode(context.Background(), "Times Square"); err != nil {
		t.Fatalf("first Geocode: %v", err)
	}

	secondProcess := NewGeocoder(cfg, server.URL, nil)
	got, err := secondProcess.Geocode(context.Background(), "  TIMES   SQUARE ")
	if err != nil {
		t.Fatalf("second Geocode: %v", err)
	}
	if got.Name != "Times Square, New York" || got.Lat != 40.757 || got.Lon != -73.986 {
		t.Fatalf("stop = %#v", got)
	}
	if requests != 1 {
		t.Fatalf("Nominatim requests = %d, want 1", requests)
	}
}
