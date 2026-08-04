package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func SetupRouteData(addresses []string, geocodeCache GeocodeCache, matrixCache MatrixCache) ([]Stop, [][]float64, error) {
	stops, err := getStops(addresses, geocodeCache)
	if err != nil {
		return nil, nil, fmt.Errorf("geocode stops: %w", err)
	}
	durations, _, err := buildDistanceMatrix(stops, matrixCache)
	if err != nil {
		return nil, nil, fmt.Errorf("build duration matrix: %w", err)
	}
	return stops, durations, nil
}

func main() {
	if err := run(); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

func run() (err error) {
	configPath := flag.String("config", "config.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.applyRuntime()

	var cliAddressesFile string
	if flag.NArg() >= 1 {
		cliAddressesFile = flag.Arg(0)
	}

	addresses, err := cfg.resolveAddresses(cliAddressesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: go run . [-config config.yaml] [addresses-file]\n")
		return fmt.Errorf("resolve addresses: %w", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("no addresses found in input")
	}
	if len(addresses) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many addresses: %d (max %d); raise solver.max_stops in config if intentional",
			len(addresses), cfg.Solver.MaxStops)
	}
	fmt.Printf("\nLoaded %d addresses (config: %s)\n", len(addresses), *configPath)

	geocodeCache, err := loadGeocode(cfg.Cache.GeocodeFile)
	if err != nil {
		return fmt.Errorf("load geocode cache %q: %w", cfg.Cache.GeocodeFile, err)
	}
	matrixCache, err := loadMatrix(cfg.Cache.MatrixFile)
	if err != nil {
		return fmt.Errorf("load matrix cache %q: %w", cfg.Cache.MatrixFile, err)
	}

	defer func() {
		if saveErr := persistCaches(cfg, geocodeCache, matrixCache); saveErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; also failed to save caches: %v", err, saveErr)
			} else {
				err = saveErr
			}
		}
	}()

	stops, durations, err := SetupRouteData(addresses, geocodeCache, matrixCache)
	if err != nil {
		return fmt.Errorf("setup route data: %w", err)
	}

	routes, err := solve(stops, durations, cfg.Solver.TopK)
	if err != nil {
		return fmt.Errorf("solve top %d routes: %w", cfg.Solver.TopK, err)
	}
	if len(routes) == 0 {
		return fmt.Errorf("solve returned no routes")
	}

	if err := printRoutes(routes, stops, cfg); err != nil {
		return err
	}
	fmt.Printf("Finish\n")
	return nil
}

func persistCaches(cfg Config, geocodeCache GeocodeCache, matrixCache MatrixCache) error {
	if err := saveGeocode(cfg.Cache.GeocodeFile, geocodeCache); err != nil {
		return fmt.Errorf("save geocode cache %q: %w", cfg.Cache.GeocodeFile, err)
	}
	if err := saveMatrix(cfg.Cache.MatrixFile, matrixCache); err != nil {
		return fmt.Errorf("save matrix cache %q: %w", cfg.Cache.MatrixFile, err)
	}
	return nil
}

func printRoutes(routes []RouteResult, stops []Stop, cfg Config) error {
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
			if idx < 0 || idx >= len(stops) {
				return fmt.Errorf("route %d contains invalid stop index %d", i+1, idx)
			}
			fmt.Printf("\t%s\n", stops[idx].Name)
		}
		fmt.Println()
	}
	return nil
}
