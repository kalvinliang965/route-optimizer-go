package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

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

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: go run main.go <string-address-input-file>\n")
		return
	}

	file_path := os.Args[1]
	fmt.Printf("\nReading stops from: %s\n", file_path)

	addresses, err := read_addresses_from_file(file_path)
	if err != nil {
		log.Fatalf("Failed to read addresses: %v\n", err)
	}

	stops, durations, findIdx, err := SetupRouteData(addresses)
	if err != nil {
		log.Fatalf("Error setting up route data: %v\n", err)
	}

	if len(stops) >= 2 {
		idx1 := findIdx(stops[0].Name)
		idx2 := findIdx(stops[1].Name)
		if idx1 != -1 && idx2 != -1 {
			fmt.Printf("Time from %s to %s: %.1f seconds\n", stops[idx1].Name, stops[idx2].Name, durations[idx1][idx2])
		}
	}
	fmt.Printf("Finish\n")
}