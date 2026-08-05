package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"route-optimizer-go/internal/route"
	"route-optimizer-go/pepsi"
)

// Execute parses os.Args and runs the selected subcommand.
func Execute() error {
	if len(os.Args) < 2 {
		Usage()
		os.Exit(2)
	}
	return Run(os.Args[1], os.Args[2:])
}

// Usage prints top-level CLI help to stderr.
func Usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  route-optimizer <command> [flags]

Commands:
  geocode     -addresses addresses.txt  →  -out stops.json (lat/lon + display name)
  matrix      -stops stops.json         →  -out matrix.json (OSRM durations)
  edge-metadata -stops stops.json       →  -out edge_metadata.json (OSRM route geometry)
  itinerary   -stops stops.json -matrix matrix.json → top-K routes + Maps links

Shared flags:
  -config   YAML config (default: config.yaml)

Examples:
  route-optimizer geocode -addresses addresses.txt -out data/stops.json
  route-optimizer matrix -stops data/stops.json -out data/matrix.json
  route-optimizer edge-metadata -stops data/stops.json -out data/edge_metadata.json
  route-optimizer itinerary -stops data/stops.json -matrix data/matrix.json
  route-optimizer itinerary -stops data/stops.json -refresh-matrix

Run 'route-optimizer <command> -h' for command-specific flags.
`)
}

// Run dispatches a subcommand with its flag args.
func Run(command string, args []string) error {
	switch command {
	case "geocode":
		return runGeocode(args)
	case "matrix":
		return runMatrix(args)
	case "edge-metadata":
		return runEdgeMetadata(args)
	case "itinerary":
		return runItinerary(args)
	default:
		Usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func runEdgeMetadata(args []string) error {
	fs := flag.NewFlagSet("edge-metadata", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	outPath := fs.String("out", "data/edge_metadata.json", "output edge metadata JSON")
	osrmBaseURL := fs.String("osrm-base-url", pepsi.DefaultOSRMRouteBaseURL, "OSRM route service base URL")
	fs.Parse(args)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	var stops []route.Stop
	if err := route.ReadJSON(*stopsPath, &stops); err != nil {
		return fmt.Errorf("read stops %s: %w", *stopsPath, err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("no stops in %s", *stopsPath)
	}
	if len(stops) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many stops: %d (max %d)", len(stops), cfg.Solver.MaxStops)
	}

	client := pepsi.NewClient(*osrmBaseURL)
	client.UserAgent = cfg.HTTP.UserAgent
	client.HTTPClient = &http.Client{
		Timeout: time.Duration(cfg.HTTP.OSRMTimeoutSec) * time.Second,
	}

	fmt.Printf("edge-metadata: building %d directed edges from %s\n", len(stops)*(len(stops)-1), *stopsPath)
	metadata, err := client.BuildEdgeMetadata(context.Background(), stops)
	if err != nil {
		return fmt.Errorf("build edge metadata: %w", err)
	}
	if err := pepsi.WriteEdgeMetadata(*outPath, metadata); err != nil {
		return fmt.Errorf("write edge metadata %s: %w", *outPath, err)
	}

	fmt.Printf("edge-metadata: wrote %s (%d edges)\n", *outPath, len(metadata.Edges))
	return nil
}

func runItinerary(args []string) error {
	fs := flag.NewFlagSet("itinerary", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	matrixPath := fs.String("matrix", "data/matrix.json", "duration matrix JSON from matrix")
	refreshMatrix := fs.Bool("refresh-matrix", false, "rebuild matrix.json from stops (uses OSRM/cache) before solving")
	fs.Parse(args)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	var stops []route.Stop
	if err := route.ReadJSON(*stopsPath, &stops); err != nil {
		return fmt.Errorf("read stops %s: %w", *stopsPath, err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("no stops in %s", *stopsPath)
	}
	if len(stops) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many stops: %d (max %d)", len(stops), cfg.Solver.MaxStops)
	}

	var matrix [][]float64
	needBuild := *refreshMatrix
	if !needBuild {
		if err := route.ReadJSON(*matrixPath, &matrix); err != nil {
			if os.IsNotExist(err) {
				needBuild = true
			} else {
				return fmt.Errorf("read matrix %s: %w", *matrixPath, err)
			}
		}
	}
	if needBuild {
		fmt.Printf("itinerary: building matrix for %d stops → %s\n", len(stops), *matrixPath)
		matrix, err = buildMatrixArtifact(stops, cfg.Cache.MatrixFile)
		if err != nil {
			return err
		}
		if err := route.WriteJSON(*matrixPath, matrix); err != nil {
			return fmt.Errorf("write matrix %s: %w", *matrixPath, err)
		}
	}

	fmt.Printf("itinerary: solving top %d from %s + %s\n", cfg.Solver.TopK, *stopsPath, *matrixPath)
	routes, err := route.Solve(stops, matrix, cfg.Solver.TopK)
	if err != nil {
		return fmt.Errorf("solve: %w", err)
	}
	return route.PrintRoutes(routes, stops, cfg)
}

func runMatrix(args []string) error {
	fs := flag.NewFlagSet("matrix", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	outPath := fs.String("out", "data/matrix.json", "output duration matrix JSON")
	fs.Parse(args)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	var stops []route.Stop
	if err := route.ReadJSON(*stopsPath, &stops); err != nil {
		return fmt.Errorf("read stops %s: %w", *stopsPath, err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("no stops in %s", *stopsPath)
	}
	if len(stops) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many stops: %d (max %d)", len(stops), cfg.Solver.MaxStops)
	}

	fmt.Printf("matrix: building %d×%d from %s\n", len(stops), len(stops), *stopsPath)
	matrix, err := buildMatrixArtifact(stops, cfg.Cache.MatrixFile)
	if err != nil {
		return err
	}

	if err := route.WriteJSON(*outPath, matrix); err != nil {
		return fmt.Errorf("write matrix %s: %w", *outPath, err)
	}

	fmt.Printf("matrix: wrote %s (%d×%d); cache updated %s\n",
		*outPath, len(matrix), len(matrix[0]), cfg.Cache.MatrixFile)
	return nil
}

func runGeocode(args []string) error {
	fs := flag.NewFlagSet("geocode", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	addressesPath := fs.String("addresses", "addresses.txt", "newline-separated addresses file")
	outPath := fs.String("out", "data/stops.json", "output stops file")
	fs.Parse(args)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	addresses, err := cfg.ResolveAddresses(*addressesPath)
	if err != nil {
		return fmt.Errorf("resolve addresses: %w", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("no addresses found in input")
	}
	if len(addresses) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many addresses: %d (max %d); raise solver.max_stops in config if intentional",
			len(addresses), cfg.Solver.MaxStops)
	}

	fmt.Printf("geocode: resolving %d addresses from %s\n", len(addresses), *addressesPath)

	cache, err := route.LoadGeocode(cfg.Cache.GeocodeFile)
	if err != nil {
		return fmt.Errorf("load geocode cache: %w", err)
	}
	stops, err := route.GetStops(addresses, cache)
	if err != nil {
		return fmt.Errorf("resolve stops: %w", err)
	}
	if err := route.SaveGeocode(cfg.Cache.GeocodeFile, cache); err != nil {
		return fmt.Errorf("save geocode cache: %w", err)
	}

	if err := route.WriteStops(*outPath, stops); err != nil {
		return fmt.Errorf("write stops %s: %w", *outPath, err)
	}

	fmt.Printf("geocode: wrote %s (%d stops); cache updated %s\n",
		*outPath, len(stops), cfg.Cache.GeocodeFile)
	return nil
}

// buildMatrixArtifact loads the growable pair cache, builds N×N for these stops, saves cache.
// Used by `matrix` and by `itinerary -refresh-matrix` (missing matrix file).
func buildMatrixArtifact(stops []route.Stop, cachePath string) ([][]float64, error) {
	cache, err := route.LoadMatrix(cachePath)
	if err != nil {
		return nil, fmt.Errorf("load matrix cache: %w", err)
	}
	matrix, _, err := route.BuildDistanceMatrix(stops, cache)
	if err != nil {
		return nil, fmt.Errorf("build duration matrix: %w", err)
	}
	if err := route.SaveMatrix(cachePath, cache); err != nil {
		return nil, fmt.Errorf("save matrix cache: %w", err)
	}
	return matrix, nil
}
