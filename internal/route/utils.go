package route

import (
  "bufio"
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "strings"
)

// SecondsToMinutes converts OSRM duration (seconds) to minutes for display.
func SecondsToMinutes(seconds float64) float64 {
  return seconds / 60.0
}

// WriteJSON marshals v as indented JSON and atomically replaces path, creating
// parent directories. Writing a sibling temporary file first keeps the last
// successful artifact intact if marshaling or writing the replacement fails.
func WriteJSON(path string, v interface{}) error {
  data, err := json.MarshalIndent(v, "", "  ")
  if err != nil {
    return err
  }
  dir := filepath.Dir(path)
  if err := os.MkdirAll(dir, 0755); err != nil {
    return err
  }

  tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
  if err != nil {
    return err
  }
  tempPath := tempFile.Name()
  keepTemp := true
  defer func() {
    if keepTemp {
      _ = tempFile.Close()
      _ = os.Remove(tempPath)
    }
  }()

  if err := tempFile.Chmod(0644); err != nil {
    return err
  }
  if _, err := tempFile.Write(data); err != nil {
    return err
  }
  if err := tempFile.Sync(); err != nil {
    return err
  }
  if err := tempFile.Close(); err != nil {
    return err
  }
  if err := os.Rename(tempPath, path); err != nil {
    return err
  }
  keepTemp = false
  return nil
}

// ReadJSON reads path and unmarshals JSON into dest.
// Returns a wrapped error if the file is missing or invalid.
func ReadJSON(path string, dest interface{}) error {
  data, err := os.ReadFile(path)
  if err != nil {
    return err
  }
  return json.Unmarshal(data, dest)
}

// WriteStops writes an ordered stop list to JSON (geocode command artifact).
func WriteStops(path string, stops []Stop) error {
  return WriteJSON(path, stops)
}

// ReadStops loads an ordered stop list from JSON.
func ReadStops(path string) ([]Stop, error) {
  var stops []Stop
  if err := ReadJSON(path, &stops); err != nil {
    return nil, fmt.Errorf("read stops %s: %w", path, err)
  }
  return stops, nil
}

func ReadAddressesFromFile(path string) ([]string, error) {
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


func GetStop(addr string, cache GeocodeCache) (*Stop, error) {
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

func GetStops(addresses []string, cache GeocodeCache) ([]Stop, error) {
  n := len(addresses)
  stops := make([]Stop, n);
  
  for i, addr := range addresses {
    s, err := GetStop(addr, cache)
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

func GetDistance(from Stop, to Stop, cache MatrixCache) (float64, error) {
  if dist, ok := lookupCachedDistance(from, to, cache); ok {
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

func BuildDistanceMatrix(stops []Stop, cache MatrixCache) ([][]float64, func(string) int, error) {
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
