package cache

import (
	"context"
	"testing"
	"time"

	"route-optimizer-go/internal/optimizer"
)

func TestFileMatrixStoreRoundTripAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	store := NewFileMatrixStore(t.TempDir(), 30*24*time.Hour)
	store.Now = func() time.Time { return now }
	key := MatrixKey("https://osrm.example/driving", []optimizer.Stop{{Lat: 40, Lon: -73}})
	want := optimizer.Matrix{{0, 12}, {15, 0}}

	if err := store.Save(context.Background(), key, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, state, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state != MatrixFresh || got[0][1] != 12 || got[1][0] != 15 {
		t.Fatalf("matrix = %#v, state = %v", got, state)
	}

	store.Now = func() time.Time { return now.Add(30 * 24 * time.Hour) }
	got, state, err = store.Load(context.Background(), key)
	if err != nil || state != MatrixStale || got[0][1] != 12 {
		t.Fatalf("expired Load matrix = %#v, state = %v, err = %v", got, state, err)
	}
}

func TestFileMatrixStoreRejectsInvalidMatrix(t *testing.T) {
	store := NewFileMatrixStore(t.TempDir(), time.Hour)
	key := MatrixKey("https://osrm.example/driving", []optimizer.Stop{{Lat: 40, Lon: -73}})

	if err := store.Save(context.Background(), key, optimizer.Matrix{{0}, {1}}); err == nil {
		t.Fatal("Save invalid matrix error = nil")
	}
}

func TestMatrixKeyIncludesProviderAndStopOrder(t *testing.T) {
	stops := []optimizer.Stop{{Name: "A", Lat: 40, Lon: -73}, {Name: "B", Lat: 41, Lon: -74}}
	original := MatrixKey("https://osrm.example", stops)
	renamed := MatrixKey("https://osrm.example", []optimizer.Stop{{Name: "Renamed", Lat: 40, Lon: -73}, {Lat: 41, Lon: -74}})
	reordered := MatrixKey("https://osrm.example", []optimizer.Stop{stops[1], stops[0]})
	otherProvider := MatrixKey("https://other.example", stops)

	if original != renamed {
		t.Fatal("names should not change the matrix cache key")
	}
	if original == reordered {
		t.Fatal("ordered coordinates must change the matrix cache key")
	}
	if original == otherProvider {
		t.Fatal("provider namespace must change the matrix cache key")
	}
}
