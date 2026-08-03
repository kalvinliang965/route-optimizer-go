package main

import (
  "bufio"
  "fmt"
  "os"
  "strings"
)

func read_addresses_from_file(path string) ([]string, error) {
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

func build_stops_from_addresses(addresses []string) ([]Stop, error) {
  var stops []Stop
  for _, addr := range addresses {
    s, err := geocodeAddress(addr)
    if err != nil {
      return nil, fmt.Errorf("Failed to build stops: %v", err)
    }
    stops = append(stops, *s)
  }
  return stops, nil
}

type OSRMTableResponse struct {
  Code      string      `json:"code"`
  Durations [][]float64 `json:"durations"`
}

