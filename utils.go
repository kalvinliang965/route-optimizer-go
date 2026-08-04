package main

import (
  "bufio"
  "fmt"
  "os"
  "strings"
)

// secondsToMinutes converts OSRM duration (seconds) to minutes for display.
func secondsToMinutes(seconds float64) float64 {
  return seconds / 60.0
}

func readAddressesFromFile(path string) ([]string, error) {
  file, err := os.Open(path)
  if err != nil {
    err_msg := fmt.Sprintf("Failed to open files %s: %v", path, err)
    return nil, fmt.Errorf(err_msg)
  }
  defer file.Close()
  scanner := bufio.NewScanner(file)
  var addresses []string
  for scanner.Scan() {
    addr := strings.TrimRight(strings.TrimSpace(scanner.Text()), "\r")
    if addr == "" {
      continue
    }
    addresses = append(addresses, addr)
  }
  if err := scanner.Err(); err != nil {
    err_msg := fmt.Sprintf("Failed to read file %s: %v", path, err)
    return nil, fmt.Errorf(err_msg)
  }
  return addresses, nil
}


func getStop(addr string, cache GeocodeCache) (*Stop, error) {
   if cachedStop, exists := cache[addr]; exists {
     fmt.Printf("Cache hit: %s\n", addr);
     return &cachedStop, nil;
   } 
    fmt.Printf("[Cache Miss] Fetching from API: %s\n", addr)
    stopPtr, err := FetchGeocodeAddress(addr)
    if err != nil {
        return nil, err
    }
    cache[addr] = *stopPtr
    return stopPtr, nil
}

func getStops(addresses []string, cache GeocodeCache) ([]Stop, error) {
  n := len(addresses)
  stops := make([]Stop, n);
  
  for i, addr := range addresses {
    s, err := getStop(addr, cache)
    if err != nil {
      return nil, fmt.Errorf("Failed to build stops: %v", err)
    }
    stops[i] = *s
  }
  return stops, nil
}

func stopCoordKey(s Stop) string {
  return fmt.Sprintf("%.6f, %.6f", s.Lat, s.Lon)
}

func lookupCachedDistance(from Stop, to Stop, cache MatrixCache) (float64, bool) {
  src := stopCoordKey(from)
  dest := stopCoordKey(to)
  if srcMap, exists := cache[src]; exists {
    if dist, ok := srcMap[dest]; ok {
      return dist, true
    }
  }
  return 0, false
}

func putCachedDistance(from Stop, to Stop, dist float64, cache MatrixCache) {
  src := stopCoordKey(from)
  dest := stopCoordKey(to)
  if _, exists := cache[src]; !exists {
    cache[src] = make(map[string]float64)
  }
  cache[src][dest] = dist
}

func getDistance(from Stop, to Stop, cache MatrixCache) (float64, error) {
  if dist, ok := lookupCachedDistance(from, to, cache); ok {
    fmt.Printf("[cache hit]")
    return dist, nil
  }
  // Pairwise fallback for single-edge lookups (tests / ad-hoc use).
  durations, err := FetchDurationMatrix([]Stop{from, to})
  if err != nil {
    return -1, err
  }
  putCachedDistance(from, to, durations[0][1], cache)
  return durations[0][1], nil
}

func matrixHasMissingEdges(stops []Stop, cache MatrixCache) bool {
  for i := range stops {
    for j := range stops {
      if i == j {
        continue
      }
      if _, ok := lookupCachedDistance(stops[i], stops[j], cache); !ok {
        return true
      }
    }
  }
  return false
}

func buildDistanceMatrix(stops []Stop, cache MatrixCache) ([][]float64, func(string) int, error) {
  n := len(stops)
  distMatrix := make([][]float64, n)
  for i := range stops {
    distMatrix[i] = make([]float64, n)
  }

  // One full-table OSRM call fills every missing edge (rate-limit friendly).
  if n > 1 && matrixHasMissingEdges(stops, cache) {
    fmt.Printf("[Cache Miss] Fetching full duration table for %d stops\n", n)
    table, err := FetchDurationMatrix(stops)
    if err != nil {
      return nil, nil, err
    }
    if len(table) != n {
      return nil, nil, fmt.Errorf("osrm table size mismatch: got %d rows, want %d", len(table), n)
    }
    for i := 0; i < n; i++ {
      if len(table[i]) != n {
        return nil, nil, fmt.Errorf("osrm table size mismatch: row %d has %d cols, want %d", i, len(table[i]), n)
      }
      for j := 0; j < n; j++ {
        if i == j {
          continue
        }
        putCachedDistance(stops[i], stops[j], table[i][j], cache)
      }
    }
  }

  for i := 0; i < n; i++ {
    for j := 0; j < n; j++ {
      if i == j {
        continue
      }
      dist, ok := lookupCachedDistance(stops[i], stops[j], cache)
      if !ok {
        return nil, nil, fmt.Errorf("missing cached duration from stop %d to %d", i, j)
      }
      distMatrix[i][j] = dist
    }
  }

  findIdx := func(query string) int {
    for i, stop := range stops {
      if strings.EqualFold(stop.Name, query) || strings.Contains(strings.ToLower(stop.Name), strings.ToLower(query)) {
        return i
      }
    }
    return -1
  }

  return distMatrix, findIdx, nil
}