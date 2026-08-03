package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type OSMTripResponse struct {
	Code      string     `json:"code"`
	Waypoints []Waypoint `json:"waypoints"`
	Trips     []Trip     `json:"trips"`
}

type Waypoint struct {
	Name          string    `json:"name"`
	Location      []float64 `json:"Location"`
	WaypointIndex int       `json:"waypoint_index"`
	TripsIndex    int       `json:"trips_index"`
}

type Trip struct {
	Geometry string  `json:"geometry"`
	Duration float64 `json:"duration"`
	Distance float64 `json:"distance"`
}

type AddressStruct struct {
	Name string `json:"display_name"`
	Lon  string `json:"lon"`
	Lat  string `json:"lat"`
}

type Stop struct {
	Name string
	Lon  float64
	Lat  float64
}

func pick_result(results []AddressStruct) AddressStruct {
	return results[0] // by default pick the first result
}

func geocodeAddress(address string) (*Stop, error) {
	endpoint := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1", url.QueryEscape(address))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "GoRouteOptimizerApp/1.0 (student project)")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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

	result := pick_result(results)

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


func fetchDurationMatrix(stops []Stop) ([][]float64, error) {
	// 1. Build the coordinate string format: "lon,lat;lon,lat;..."
	var coordStrs []string
	for _, s := range stops {
		coordStrs = append(coordStrs, fmt.Sprintf("%f,%f", s.Lon, s.Lat))
	}
	coordinatesParam := strings.Join(coordStrs, ";")

	// 2. Construct the OSRM Table API endpoint URL
	apiURL := fmt.Sprintf("http://router.project-osrm.org/table/v1/driving/%s?annotations=duration", coordinatesParam)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoRouteOptimizerApp/1.0 (student project)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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

// SetupRouteData processes stops and returns the duration matrix and a dedicated index lookup function
func SetupRouteData(addresses []string) ([]Stop, [][]float64, func(string) int, error) {
	stops, err := build_stops_from_addresses(addresses)
	if err != nil {
		return nil, nil, nil, err
	}
	durations, err := fetchDurationMatrix(stops)
	if err != nil {
		return nil, nil, nil, err
	}
	findIdx := func(query string) int {
		for i, stop := range stops {
			if strings.EqualFold(stop.Name, query) || strings.Contains(strings.ToLower(stop.Name), strings.ToLower(query)) {
				return i
			}
		}
		return -1
	}
	return stops, durations, findIdx, nil
}

func cache_matrix_file() {

	
}

func load_matrix() {

}


func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: go run main <string-address-input-file>\n");
		return;
	}

	file_path := os.Args[1];

	fmt.Printf("\n\n\nReading stops from: %s\n", file_path);

	addresses, err := read_addresses_from_file(file_path);
	if err != nil {
		log.Fatalf("Failed to read addresses from file %s: %v\n", file_path, err);
	}

	fmt.Printf("Parsing addresses:\n")
	for i, addr := range addresses {
		fmt.Printf("[%d] %q\n", i, addr);
	}

	stops, err := build_stops_from_addresses(addresses);
	if err != nil {
		fmt.Printf("Failed to build stops from provided addresses: %v\n", err);
		return;
	}

	fmt.Printf("Parsed geocode stops: %v\n", stops);

	stops, durations, findIdx, err := SetupRouteData(addresses)
	if err != nil {
			log.Fatalf("Error setting up route data: %v", err)
	}

	if len(stops) >= 2 {
			firstAddr := stops[0].Name;
			secondAddr := stops[1].Name;

			idx1 := findIdx(firstAddr)
			idx2 := findIdx(secondAddr)

			if idx1 != -1 && idx2 != -1 {
					fmt.Printf("Time from %s to %s: %.1f seconds\n", 
							stops[idx1].Name, stops[idx2].Name, durations[idx1][idx2])
			} else {
					fmt.Printf("Could not resolve indices for the first two addresses.\n")
			}
	} else {
			fmt.Printf("Not enough addresses provided in the input file to run the pairwise demo.\n")
	}

	fmt.Printf("Finish\n\n\n")
}
