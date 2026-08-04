package cli

import (
	"flag"
	"fmt"
	"os"

	"route-optimizer-go/internal/route"
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
  geocode     Resolve addresses.txt into stops.json (lat/lon + display name)
  matrix      Build OSRM duration matrix from stops.json
  itinerary   Compute top-K routes and print Google Maps links

Examples:
  route-optimizer geocode -in addresses.txt -out data/stops.json
  route-optimizer matrix -stops data/stops.json -out data/matrix.json
  route-optimizer matrix -stops data/stops.json -refresh
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
	case "itinerary":
		return fmt.Errorf("itinerary command not implemented yet")
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runMatrix(args []string) error {
	fs := flag.NewFlagSet("matrix", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	inPath := fs.String("in", "data/stops.json", "stops file")
	outPath := fs.String("out", "data/matrix.json", "distance matrix file")
	fs.Parse(args)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	var stops []route.Stop
	if err := route.ReadJSON(*inPath, &stops); err != nil {
		return fmt.Errorf("read stops %s: %w", *inPath, err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("no stops in %s", *inPath)
	}
	if len(stops) > cfg.Solver.MaxStops {
		return fmt.Errorf("too many stops: %d (max %d)", len(stops), cfg.Solver.MaxStops)
	}

	matrixCache, err := route.LoadMatrix(cfg.Cache.MatrixFile)
	if err != nil {
		return fmt.Errorf("load matrix cache: %w", err)
	}

	fmt.Printf("matrix: building %d×%d from %s\n", len(stops), len(stops), *inPath)
	matrix, _, err := route.BuildDistanceMatrix(stops, matrixCache)
	if err != nil {
		return fmt.Errorf("build duration matrix: %w", err)
	}

	if err := route.SaveMatrix(cfg.Cache.MatrixFile, matrixCache); err != nil {
		return fmt.Errorf("save matrix cache: %w", err)
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
	inPath := fs.String("in", "addresses.txt", "addresses file")
	outPath := fs.String("out", "data/stops.json", "output stops file")
	fs.Parse(args)

	fmt.Println("running geocode")
	fmt.Println("config:", *configPath)
	fmt.Println("in:", *inPath)
	fmt.Println("out:", *outPath)

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	addresses, err := cfg.ResolveAddresses(*inPath)
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
	fmt.Printf("\nLoaded %d addresses (config: %s)\n", len(addresses), *configPath)

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

	fmt.Printf("Wrote %d stops to %s\n", len(stops), *outPath)
	return nil
}
