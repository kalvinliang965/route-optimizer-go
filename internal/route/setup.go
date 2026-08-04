package route

import "fmt"

// SetupRouteData geocodes addresses and builds the OSRM duration matrix.
func SetupRouteData(addresses []string, geocodeCache GeocodeCache, matrixCache MatrixCache) ([]Stop, [][]float64, error) {
	stops, err := GetStops(addresses, geocodeCache)
	if err != nil {
		return nil, nil, fmt.Errorf("geocode stops: %w", err)
	}
	durations, _, err := BuildDistanceMatrix(stops, matrixCache)
	if err != nil {
		return nil, nil, fmt.Errorf("build duration matrix: %w", err)
	}
	return stops, durations, nil
}

// PersistCaches writes geocode and matrix caches to disk.
func PersistCaches(cfg Config, geocodeCache GeocodeCache, matrixCache MatrixCache) error {
	if err := SaveGeocode(cfg.Cache.GeocodeFile, geocodeCache); err != nil {
		return fmt.Errorf("save geocode cache %q: %w", cfg.Cache.GeocodeFile, err)
	}
	if err := SaveMatrix(cfg.Cache.MatrixFile, matrixCache); err != nil {
		return fmt.Errorf("save matrix cache %q: %w", cfg.Cache.MatrixFile, err)
	}
	return nil
}

// PrintRoutes prints ranked routes using config duration units.
func PrintRoutes(routes []RouteResult, stops []Stop, cfg Config) error {
	fmt.Printf("\nTop %d routes\n=====================\n", cfg.Solver.TopK)
	for i, routeRes := range routes {
		total := routeRes.Duration
		unit := "secs"
		if cfg.Output.DurationUnit == "minutes" {
			total = SecondsToMinutes(routeRes.Duration)
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
