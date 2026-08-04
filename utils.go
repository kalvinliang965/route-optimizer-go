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

func getDistance(from Stop, to Stop, cache MatrixCache) (float64, error) {

  src := fmt.Sprintf("%.6f, %.6f", from.Lat, from.Lon);
  dest := fmt.Sprintf("%.6f, %.6f", to.Lat, to.Lon);

  if srcMap, exists := cache[src]; exists {
    if dist, exists := srcMap[dest]; exists {
      fmt.Printf("[cache hit]");
      return dist, nil;
    }
  }
  durations, err := FetchDurationMatrix([]Stop{from, to})
  if err != nil {
    return -1, err
  }
  if _, exists := cache[src]; !exists {
    cache[src] = make(map[string]float64)
  }
  cache[src][dest] = durations[0][1]
  return cache[src][dest], err
}

func buildDistanceMatrix(stops []Stop, cache MatrixCache) ([][]float64, func(string) int, error) {
  n := len(stops)
  distMatrix := make([][]float64, n)
  for i := range stops {
    distMatrix[i] = make([]float64, n)
  }

  for i := 0; i < n; i++ {
    for j := 0; j < n; j++ {
      if i == j {
        continue
      }
      dist, err := getDistance(stops[i], stops[j], cache);
      if err != nil {
        return nil, nil, err
      }
      distMatrix[i][j] = dist;
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
  
  return distMatrix,findIdx, nil
}