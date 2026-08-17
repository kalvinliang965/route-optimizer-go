package cache

import (
	"context"
	"testing"
	"time"

	"route-optimizer-go/internal/optimizer"
)

func TestFileGeocodeStoreRoundTripAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	store := NewFileGeocodeStore(t.TempDir(), 90*24*time.Hour)
	store.Now = func() time.Time { return now }
	key := GeocodeKey("https://nominatim.example/search", "Times Square")
	toSave := optimizer.Stop{ID: "request-specific", Name: "Times Square, New York", Lat: 40.757, Lon: -73.986}
	want := optimizer.Stop{Name: "Times Square, New York", Lat: 40.757, Lon: -73.986}

	if err := store.Save(context.Background(), key, toSave); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, state, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state != GeocodeFresh || got != want {
		t.Fatalf("stop = %#v, state = %v", got, state)
	}

	store.Now = func() time.Time { return now.Add(90 * 24 * time.Hour) }
	got, state, err = store.Load(context.Background(), key)
	if err != nil || state != GeocodeStale || got != want {
		t.Fatalf("expired Load stop = %#v, state = %v, err = %v", got, state, err)
	}
}

func TestGeocodeKeyNormalizesAddressAndIncludesProvider(t *testing.T) {
	original := GeocodeKey("https://nominatim.example", "  Times   Square ")
	normalized := GeocodeKey("https://nominatim.example/", "times square")
	otherAddress := GeocodeKey("https://nominatim.example", "Union Square")
	otherProvider := GeocodeKey("https://other.example", "Times Square")

	if original != normalized {
		t.Fatal("address capitalization and whitespace should not change the geocode cache key")
	}
	if original == otherAddress {
		t.Fatal("address must change the geocode cache key")
	}
	if original == otherProvider {
		t.Fatal("provider namespace must change the geocode cache key")
	}
}

func TestFileGeocodeStoreRejectsInvalidStop(t *testing.T) {
	store := NewFileGeocodeStore(t.TempDir(), time.Hour)
	key := GeocodeKey("https://nominatim.example", "Unknown")

	if err := store.Save(context.Background(), key, optimizer.Stop{Name: "Invalid", Lat: 91}); err == nil {
		t.Fatal("Save invalid stop error = nil")
	}
}
