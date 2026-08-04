package main

import (
	"errors"
	"strings"
	"testing"
)

// TestSetupRouteData_PropagatesMatrixError ensures matrix-build failures
// are returned to the caller (currently swallowed as nil).
func TestSetupRouteData_PropagatesMatrixError(t *testing.T) {
	originalFetcher := FetchDurationMatrix
	defer func() { FetchDurationMatrix = originalFetcher }()

	wantErr := errors.New("osrm unavailable")
	FetchDurationMatrix = func(stops []Stop) ([][]float64, error) {
		return nil, wantErr
	}

	addresses := []string{"Depot St", "Stop A"}
	geocodeCache := GeocodeCache{
		"Depot St": {Name: "Depot", Lat: 40.71, Lon: -74.00},
		"Stop A":   {Name: "A", Lat: 40.72, Lon: -74.01},
	}
	matrixCache := make(MatrixCache)

	_, _, err := SetupRouteData(addresses, geocodeCache, matrixCache)
	if err == nil {
		t.Fatal("expected matrix build error to be returned, got nil")
	}
	if !errors.Is(err, wantErr) && !strings.Contains(err.Error(), wantErr.Error()) {
		t.Errorf("expected error wrapping %q, got %v", wantErr, err)
	}
}
