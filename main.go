package main

import (
	"fmt"
	"log"
	"os"
)

func SetupRouteData(addresses []string, geocodeCache GeocodeCache, matrixCache MatrixCache) ([]Stop, [][]float64, func(string) int, error) {
	stops, err := getStops(addresses, geocodeCache)
	if err != nil {
		return nil, nil, nil, err
	}
	durations, findIdx, err := buildDistanceMatrix(stops, matrixCache)
	return stops, durations, findIdx, nil
}

const (
		geocode_cache_file = "data/geocode_cache.json"
		matrix_cache_file = "data/matrix_cache.json"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: go run main.go <string-address-input-file>\n")
		return
	}

	file_path := os.Args[1]
	fmt.Printf("\nReading stops from: %s\n", file_path)

	addresses, err := readAddressesFromFile(file_path)
	if err != nil {
		log.Fatalf("Failed to read addresses: %v\n", err)
	}

	geocodeCache, err := loadGeocode(geocode_cache_file);
	if err != nil {
		log.Fatalf("Failed to initialize geocode cache: %v", err)	
	}
	
	matrixCache, err := loadMatrix(matrix_cache_file);
	if err != nil {
		log.Fatalf("Failed to initialize matrix cache: %v", err)
	}
	
	stops, durations, findIdx, err := SetupRouteData(addresses, geocodeCache, matrixCache)
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


	err = saveGeocode(geocode_cache_file, geocodeCache)
	if err != nil {
		log.Fatalf("Failed to save geocode cache: %v", err);
	}
	
	err = saveMatrix(matrix_cache_file, matrixCache)
	if err != nil {
		log.Fatalf("Failed to save matrix cache: %v", err);
	}
	
	fmt.Printf("Finish\n")
}
