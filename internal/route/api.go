package route

import (
  "encoding/json"
  "fmt"
  "io"
  "net/http"
  "net/url"
  "strconv"
  "strings"
  "time"
)

func pickResult(results []AddressStruct) AddressStruct {
  return results[0] // by default pick the first result
}

// Overridden from YAML via Config.ApplyRuntime / tests (httptest).
var (
  GeocodeTimeout   = 5 * time.Second
  OSRMTimeout      = 10 * time.Second
  HTTPUserAgent    = "GoRouteOptimizerApp/1.0 (student project)"
  NominatimBaseURL = "https://nominatim.openstreetmap.org"
  OSRMTableBaseURL = "http://router.project-osrm.org"
)

var FetchGeocodeAddress = func(address string) (*Stop, error) {
  endpoint := fmt.Sprintf("%s/search?q=%s&format=json&limit=1", NominatimBaseURL, url.QueryEscape(address))
  req, err := http.NewRequest("GET", endpoint, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Set("User-Agent", HTTPUserAgent)

  client := &http.Client{Timeout: GeocodeTimeout}
  resp, err := client.Do(req)
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()

  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return nil, err
  }

  if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("nominatim geocode request failed: %s", resp.Status)
  }

  var results []AddressStruct
  if err := json.Unmarshal(body, &results); err != nil {
    return nil, fmt.Errorf("failed to parse JSON: %v", err)
  }

  if len(results) == 0 {
    return nil, fmt.Errorf("no geocode address results found in response for %s", address)
  }

  fmt.Printf("Available results for %s: \n", address)
  for _, addr := range results {
    fmt.Printf("		`%s`\n", addr.Name)
  }

  result := pickResult(results)

  if len(result.Name) == 0 {
    return nil, fmt.Errorf("result chosen for `%s` is empty", address)
  }

  lat, err := strconv.ParseFloat(result.Lat, 64)
  if err != nil {
    return nil, fmt.Errorf("Invalid latitude format: `%s`: %v", result.Lat, err)
  }

  lon, err := strconv.ParseFloat(result.Lon, 64)
  if err != nil {
    return nil, fmt.Errorf("Invalid longitude format: `%s`: %v", result.Lon, err)
  }

  return &Stop{
    Name: result.Name,
    Lat:  lat,
    Lon:  lon,
  }, nil
}



var FetchDurationMatrix = func(stops []Stop) ([][]float64, error) {
  var coordStrs []string
  for _, s := range stops {
    coordStrs = append(coordStrs, fmt.Sprintf("%f,%f", s.Lon, s.Lat))
  }
  coordinatesParam := strings.Join(coordStrs, ";")

  apiURL := fmt.Sprintf("%s/table/v1/driving/%s?annotations=duration", OSRMTableBaseURL, coordinatesParam)

  req, err := http.NewRequest("GET", apiURL, nil)
  if err != nil {
    return nil, err
  }
  req.Header.Set("User-Agent", HTTPUserAgent)

  client := &http.Client{Timeout: OSRMTimeout}
  resp, err := client.Do(req)
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()

  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return nil, err
  }

  if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("osrm table request failed: %s", resp.Status)
  }

  var tableResp OSRMTableResponse
  if err := json.Unmarshal(body, &tableResp); err != nil {
    return nil, fmt.Errorf("failed to parse table JSON: %v", err)
  }

  if tableResp.Code != "Ok" {
    return nil, fmt.Errorf("osrm table api error code: %s", tableResp.Code)
  }

  return tableResp.Durations, nil
}