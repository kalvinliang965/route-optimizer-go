package main

import (
	"flag"
	"fmt"
	"log"
)

func SetupRouteData(addresses []string, geocodeCache GeocodeCache, matrixCache MatrixCache) ([]Stop, [][]float64, error) {
	stops, err := getStops(addresses, geocodeCache)
	if err != nil {
		return nil, nil, err
	}
	durations, _, err := buildDistanceMatrix(stops, matrixCache)
	return stops, durations, err
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.applyRuntime()

	var cliAddressesFile string
	if flag.NArg() >= 1 {
		cliAddressesFile = flag.Arg(0)
	}

	addresses, err := cfg.resolveAddresses(cliAddressesFile)
	if err != nil {
		fmt.Printf("Usage: go run . [-config config.yaml] [addresses-file]\n")
		log.Fatalf("Failed to resolve addresses: %v", err)
	}
	fmt.Printf("\nLoaded %d addresses (config: %s)\n", len(addresses), *configPath)

	geocodeCache, err := loadGeocode(cfg.Cache.GeocodeFile)
	if err != nil {
		log.Fatalf("Failed to initialize geocode cache: %v", err)
	}

	matrixCache, err := loadMatrix(cfg.Cache.MatrixFile)
	if err != nil {
		log.Fatalf("Failed to initialize matrix cache: %v", err)
	}

	defer func() {
		if err := saveGeocode(cfg.Cache.GeocodeFile, geocodeCache); err != nil {
			log.Fatalf("Failed to save geocode cache: %v", err)
		}
		if err := saveMatrix(cfg.Cache.MatrixFile, matrixCache); err != nil {
			log.Fatalf("Failed to save matrix cache: %v", err)
		}
	}()

	stops, durations, err := SetupRouteData(addresses, geocodeCache, matrixCache)
	if err != nil {
		log.Fatalf("Error setting up route data: %v\n", err)
	}

	routes, err := solve(stops, durations, cfg.Solver.TopK)
	if err != nil {
		log.Fatalf("Failed to solve top %d routes: %v\n", cfg.Solver.TopK, err)
	}

	fmt.Printf("\nTop %d routes\n=====================\n", cfg.Solver.TopK)
	for i, routeRes := range routes {
		total := routeRes.Duration
		unit := "secs"
		if cfg.Output.DurationUnit == "minutes" {
			total = secondsToMinutes(routeRes.Duration)
			unit = "mins"
		}
		fmt.Printf("Rank %d (Total: %.2f %s):\n", i+1, total, unit)
		for _, idx := range routeRes.Path {
			fmt.Printf("\t%s\n", stops[idx].Name)
		}
		fmt.Println()
	}

	fmt.Printf("Finish\n")
}
