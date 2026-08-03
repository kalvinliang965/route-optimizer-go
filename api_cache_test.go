// api_cache_test.go
package main

import (
  "testing"
)

func TestGetStop_CacheMissAndHit(t *testing.T) {
  // 1. Mock the Geocode API fetcher
  originalFetcher := FetchGeocodeAddress
  defer func() { FetchGeocodeAddress = originalFetcher }() // restore after test

  apiCallCount := 0
  FetchGeocodeAddress = func(addr string) (*Stop, error) {
    apiCallCount++
    return &Stop{Lat: 40.7128, Lon: -74.0060}, nil
  }

  cache := make(GeocodeCache)
  addr := "1280 Lexington Ave, New York, NY"

  // --- TEST 1: Cache Miss ---
  stop, err := getStop(addr, cache)
  if err != nil {
    t.Fatalf("unexpected error on cache miss: %v", err)
  }

  if apiCallCount != 1 {
    t.Errorf("expected 1 API call on cache miss, got %d", apiCallCount)
  }

  if stop.Lat != 40.7128 {
    t.Errorf("expected lat 40.7128, got %f", stop.Lat)
  }

  // Verify it was successfully written to the cache map
  if _, exists := cache[addr]; !exists {
    t.Errorf("expected address to be saved in cache")
  }

  // --- TEST 2: Cache Hit ---
  stop2, err := getStop(addr, cache)
  if err != nil {
    t.Fatalf("unexpected error on cache hit: %v", err)
  }

  // API call count should NOT increase because it should pull straight from cache
  if apiCallCount != 1 {
    t.Errorf("expected API call count to remain 1 on cache hit, but got %d", apiCallCount)
  }

  if stop2.Lat != 40.7128 {
    t.Errorf("expected cached lat 40.7128, got %f", stop2.Lat)
  }
}

func TestGetDistance_CacheMissAndHit(t *testing.T) {
  // 1. Mock the Duration Matrix API fetcher
  originalFetcher := FetchDurationMatrix
  defer func() { FetchDurationMatrix = originalFetcher }()

  apiCallCount := 0
  FetchDurationMatrix = func(stops []Stop) ([][]float64, error) {
    apiCallCount++
    // OSRM returns a 2x2 matrix for 2 stops
    matrix := [][]float64{
      {0.0, 150.5},
      {150.5, 0.0},
    }
    return matrix, nil
  }

  cache := make(MatrixCache)
  from := Stop{Lat: 40.7128, Lon: -74.0060}
  to := Stop{Lat: 40.7589, Lon: -73.9851}

  // --- TEST 1: Cache Miss ---
  dist, err := getDistance(from, to, cache)
  if err != nil {
    t.Fatalf("unexpected error on distance cache miss: %v", err)
  }

  if apiCallCount != 1 {
    t.Errorf("expected 1 API call, got %d", apiCallCount)
  }

  if dist != 150.5 {
    t.Errorf("expected distance 150.5, got %f", dist)
  }

  // --- TEST 2: Cache Hit ---
  dist2, err := getDistance(from, to, cache)
  if err != nil {
    t.Fatalf("unexpected error on distance cache hit: %v", err)
  }

  // API count should stay at 1
  if apiCallCount != 1 {
    t.Errorf("expected API call count to remain 1 on cache hit, but got %d", apiCallCount)
  }

  if dist2 != 150.5 {
    t.Errorf("expected cached distance 150.5, got %f", dist2)
  }
}

func TestFindIdxLogic(t *testing.T) {
  // 1. Prepare mock stops with names
  stops := []Stop{
    {Name: "Grand Central Terminal", Lat: 40.7527, Lon: -73.9772},
    {Name: "Times Square", Lat: 40.7580, Lon: -73.9855},
    {Name: "Empire State Building", Lat: 40.7484, Lon: -73.9857},
  }

  // Matrix cache doesn't need data for this test since findIdx only checks stops
  cache := make(MatrixCache)

  // 2. Call buildDistanceMatrix to get the findIdx closure back
  _, findIdx, err := buildDistanceMatrix(stops, cache)
  if err != nil {
    t.Fatalf("unexpected error building distance matrix: %v", err)
  }

  // 3. Define table-driven test cases for findIdx
  tests := []struct {
    name        string
    queryString string
    expectedIdx int
  }{
    {
      name:        "exact match",
      queryString: "Times Square",
      expectedIdx: 1,
    },
    {
      name:        "case-insensitive match",
      queryString: "GRAND CENTRAL TERMINAL",
      expectedIdx: 0,
    },
    {
      name:        "substring match",
      queryString: "Empire State",
      expectedIdx: 2,
    },
    {
      name:        "lowercase substring match",
      queryString: "terminal",
      expectedIdx: 0,
    },
    {
      name:        "no match returns -1",
      queryString: "Brooklyn Bridge",
      expectedIdx: -1,
    },
  }

  // 4. Run through the test cases
  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      gotIdx := findIdx(tt.queryString)
      if gotIdx != tt.expectedIdx {
        t.Errorf("findIdx(%q) = %d, want %d", tt.queryString, gotIdx, tt.expectedIdx)
      }
    })
  }
}