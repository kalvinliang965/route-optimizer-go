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

	defer func() {
		err = saveGeocode(geocode_cache_file, geocodeCache)
		if err != nil {
			log.Fatalf("Failed to save geocode cache: %v", err);
		}
		err = saveMatrix(matrix_cache_file, matrixCache)
		if err != nil {
			log.Fatalf("Failed to save matrix cache: %v", err);
		}
	}()
	
	stops, durations, findIdx, err := SetupRouteData(addresses, geocodeCache, matrixCache)
	if err != nil {
		log.Fatalf("Error setting up route data: %v\n", err)
	}

	top_5_route, err := solve(stops, durations, findIdx, 5)
	if err != nil {
		log.Fatalf("Failed to solve top 5 route: %v\n", err)
	}

	fmt.Printf("\nTop 5 route we calculate\n=====================\n")
	for i, routeRes := range top_5_route {
		fmt.Printf("Rank %d (Total: %.2f mins):\n", i+1, routeRes.Duration)
		for _, idx := range routeRes.Path {
			fmt.Printf("\t%s\n", stops[idx].Name)
		}
		fmt.Println()
	}
	
	fmt.Printf("Finish\n")
}
