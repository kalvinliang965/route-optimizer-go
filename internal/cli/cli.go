// Package cli is a thin command-line adapter over planner.Service.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"route-optimizer-go/internal/config"
	"route-optimizer-go/internal/geocode"
	"route-optimizer-go/internal/maps"
	"route-optimizer-go/internal/matrix"
	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
	"route-optimizer-go/internal/storage"
)

func Execute() error {
	return RunArgs(os.Args[1:])
}

func RunArgs(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" {
		printUsage(os.Stdout)
		return nil
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printUsage(os.Stdout)
			return nil
		}
		if len(args) != 2 || !isCommand(args[1]) {
			return fmt.Errorf("unknown help topic %q", args[1:])
		}
		return Run(args[1], []string{"--help"})
	}
	if !isCommand(args[0]) {
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
	return Run(args[0], args[1:])
}

func Run(command string, args []string) error {
	var err error
	switch command {
	case "all":
		err = runAll(args)
	case "geocode":
		err = runGeocode(args)
	case "matrix":
		err = runMatrix(args)
	case "optimize":
		err = runOptimize(args)
	case "itinerary":
		err = runItinerary(args)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func isCommand(command string) bool {
	switch command {
	case "all", "geocode", "matrix", "optimize", "itinerary":
		return true
	default:
		return false
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprint(writer, `Usage:
  route-optimizer all [flags] [addresses-file]
  route-optimizer <command> [flags]
  route-optimizer help [command]

Commands:
  all        Geocode, build a matrix, and calculate top-K route orders
  geocode    Resolve an address file into stops JSON
  matrix     Build an OSRM duration matrix for stops JSON
  optimize   Rank top-K round trips and write an optimization result
  itinerary  Turn a ranked optimization result into a Maps itinerary

Examples:
  route-optimizer all -top-k 5 examples/addresses.txt
  route-optimizer optimize -stops data/stops.json -matrix data/matrix.json -out data/optimization.json -top-k 5
  route-optimizer itinerary -plan data/optimization.json -rank 1
`)
}

func newFlagSet(name, invocation, summary string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.Usage = func() {
		fmt.Fprintf(set.Output(), "Usage:\n  route-optimizer %s\n\n%s\n\nFlags:\n", invocation, summary)
		set.PrintDefaults()
	}
	return set
}

func parseFlags(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			set.SetOutput(os.Stdout)
			set.Usage()
			return flag.ErrHelp
		}
		return fmt.Errorf("parse %s flags: %w", set.Name(), err)
	}
	return nil
}

func rejectPositionals(set *flag.FlagSet) error {
	if set.NArg() > 0 {
		return fmt.Errorf("%s does not accept positional arguments %q", set.Name(), set.Args())
	}
	return nil
}

func runAll(args []string) error {
	set := newFlagSet("all", "all [flags] [addresses-file]", "Run the complete route-order calculator workflow.")
	configPath := set.String("config", "config.yaml", "path to YAML config")
	addressesPath := set.String("addresses", "", "newline-separated addresses file")
	stopsOut := set.String("stops-out", "data/stops.json", "output stops JSON")
	matrixOut := set.String("matrix-out", "data/matrix.json", "output matrix JSON")
	planOut := set.String("plan-out", "data/optimization.json", "output optimization result JSON")
	topK := set.Int("top-k", 0, "number of routes to return; 0 uses config default")
	nominatimBaseURL := set.String("nominatim-base-url", "", "override Nominatim base URL")
	osrmBaseURL := set.String("osrm-base-url", "", "override OSRM base URL")
	if err := parseFlags(set, args); err != nil {
		return err
	}
	if set.NArg() > 1 {
		return fmt.Errorf("expected at most one addresses file")
	}
	if set.NArg() == 1 {
		if *addressesPath != "" {
			return fmt.Errorf("addresses file supplied both positionally and with -addresses")
		}
		*addressesPath = set.Arg(0)
	}
	if *addressesPath == "" {
		return fmt.Errorf("an addresses file is required")
	}

	cfg, selectedPath, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	addresses, err := storage.ReadAddresses(*addressesPath)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return fmt.Errorf("no addresses found in %s", *addressesPath)
	}

	service := buildService(cfg, firstNonEmpty(*nominatimBaseURL, cfg.Providers.NominatimBaseURL), firstNonEmpty(*osrmBaseURL, cfg.Providers.OSRMBaseURL))
	fmt.Printf("all: addresses → geocode → matrix → top-K calculator\n")
	if selectedPath != *configPath {
		fmt.Printf("all: %s not found; using %s\n", *configPath, selectedPath)
	}
	result, err := service.Optimize(context.Background(), planner.OptimizeRequest{Addresses: addresses, TopK: *topK})
	if err != nil {
		return fmt.Errorf("optimize: %w", err)
	}
	if err := storage.WriteJSON(*stopsOut, result.Stops); err != nil {
		return fmt.Errorf("write stops: %w", err)
	}
	if err := storage.WriteJSON(*matrixOut, result.Matrix); err != nil {
		return fmt.Errorf("write matrix: %w", err)
	}
	if err := storage.WriteJSON(*planOut, result); err != nil {
		return fmt.Errorf("write optimization result: %w", err)
	}
	itinerary, err := buildItinerary(result, 0)
	if err != nil {
		return fmt.Errorf("build itinerary: %w", err)
	}
	printResult(itinerary, cfg.Output.DurationUnit)
	fmt.Printf("all: wrote %s, %s, and %s\n", *stopsOut, *matrixOut, *planOut)
	return nil
}

func runGeocode(args []string) error {
	set := newFlagSet("geocode", "geocode [flags]", "Resolve addresses into ordered stops JSON.")
	configPath := set.String("config", "config.yaml", "path to YAML config")
	addressesPath := set.String("addresses", "", "newline-separated addresses file")
	outPath := set.String("out", "data/stops.json", "output stops JSON")
	baseURL := set.String("nominatim-base-url", "", "override Nominatim base URL")
	if err := parseFlags(set, args); err != nil {
		return err
	}
	if err := rejectPositionals(set); err != nil {
		return err
	}
	if *addressesPath == "" {
		return fmt.Errorf("-addresses is required")
	}
	cfg, _, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	addresses, err := storage.ReadAddresses(*addressesPath)
	if err != nil {
		return err
	}
	client := geocode.NewNominatim(firstNonEmpty(*baseURL, cfg.Providers.NominatimBaseURL), cfg.HTTP.UserAgent, time.Duration(cfg.HTTP.GeocodeTimeoutSec)*time.Second)
	stops := make([]optimizer.Stop, len(addresses))
	for index, address := range addresses {
		stop, err := client.Geocode(context.Background(), address)
		if err != nil {
			return fmt.Errorf("geocode address %d: %w", index, err)
		}
		stop.ID = fmt.Sprintf("stop-%d", index)
		stops[index] = stop
	}
	if err := storage.WriteJSON(*outPath, stops); err != nil {
		return err
	}
	fmt.Printf("geocode: wrote %s (%d stops)\n", *outPath, len(stops))
	return nil
}

func runMatrix(args []string) error {
	set := newFlagSet("matrix", "matrix [flags]", "Build an OSRM duration matrix for stops JSON.")
	configPath := set.String("config", "config.yaml", "path to YAML config")
	stopsPath := set.String("stops", "data/stops.json", "input stops JSON")
	outPath := set.String("out", "data/matrix.json", "output matrix JSON")
	baseURL := set.String("osrm-base-url", "", "override OSRM base URL")
	if err := parseFlags(set, args); err != nil {
		return err
	}
	if err := rejectPositionals(set); err != nil {
		return err
	}
	cfg, _, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	var stops []optimizer.Stop
	if err := storage.ReadJSON(*stopsPath, &stops); err != nil {
		return err
	}
	provider := matrix.NewOSRM(firstNonEmpty(*baseURL, cfg.Providers.OSRMBaseURL), cfg.HTTP.UserAgent, time.Duration(cfg.HTTP.MatrixTimeoutSec)*time.Second)
	durations, err := provider.Durations(context.Background(), stops)
	if err != nil {
		return err
	}
	if err := storage.WriteJSON(*outPath, durations); err != nil {
		return err
	}
	fmt.Printf("matrix: wrote %s (%d×%d)\n", *outPath, len(durations), len(durations))
	return nil
}

func runOptimize(args []string) error {
	set := newFlagSet("optimize", "optimize [flags]", "Rank top-K round trips and write an optimization result artifact.")
	configPath := set.String("config", "config.yaml", "path to YAML config")
	stopsPath := set.String("stops", "data/stops.json", "input stops JSON")
	matrixPath := set.String("matrix", "data/matrix.json", "input matrix JSON")
	outPath := set.String("out", "data/optimization.json", "output optimization result JSON")
	topK := set.Int("top-k", 0, "number of routes to return; 0 uses config default")
	if err := parseFlags(set, args); err != nil {
		return err
	}
	if err := rejectPositionals(set); err != nil {
		return err
	}
	cfg, _, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	var stops []optimizer.Stop
	if err := storage.ReadJSON(*stopsPath, &stops); err != nil {
		return err
	}
	var durations optimizer.Matrix
	if err := storage.ReadJSON(*matrixPath, &durations); err != nil {
		return err
	}
	service := planner.Service{
		Solver:      optimizer.NewSolver(cfg.Planner.MaxStops, cfg.Planner.MaxTopK),
		DefaultTopK: cfg.Planner.DefaultTopK,
	}
	result, err := service.Optimize(context.Background(), planner.OptimizeRequest{
		Stops: stops, DurationMatrixSeconds: durations, TopK: *topK,
	})
	if err != nil {
		return err
	}
	if err := storage.WriteJSON(*outPath, result); err != nil {
		return fmt.Errorf("write optimization result: %w", err)
	}
	printResult(result, cfg.Output.DurationUnit)
	fmt.Printf("optimize: wrote %s\n", *outPath)
	return nil
}

func runItinerary(args []string) error {
	set := newFlagSet("itinerary", "itinerary [flags]", "Build Maps output from an existing optimization result.")
	configPath := set.String("config", "config.yaml", "path to YAML config")
	planPath := set.String("plan", "data/optimization.json", "input optimization result JSON")
	rank := set.Int("rank", 1, "route rank to present; 0 presents every ranked route")
	if err := parseFlags(set, args); err != nil {
		return err
	}
	if err := rejectPositionals(set); err != nil {
		return err
	}
	if *rank < 0 {
		return fmt.Errorf("rank must be >= 0, got %d", *rank)
	}
	cfg, _, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	var result planner.OptimizeResult
	if err := storage.ReadJSON(*planPath, &result); err != nil {
		return err
	}
	itinerary, err := buildItinerary(result, *rank)
	if err != nil {
		return err
	}
	printResult(itinerary, cfg.Output.DurationUnit)
	return nil
}

func buildItinerary(result planner.OptimizeResult, rank int) (planner.OptimizeResult, error) {
	selected := append([]planner.PlannedRoute(nil), result.Routes...)
	if rank > 0 {
		selected = nil
		for _, route := range result.Routes {
			if route.Rank == rank {
				selected = append(selected, route)
				break
			}
		}
		if len(selected) == 0 {
			return planner.OptimizeResult{}, fmt.Errorf("route rank %d not found", rank)
		}
	}

	builder := maps.Google{}
	for index := range selected {
		url, err := builder.DirectionsURL(result.Stops, selected[index].Path)
		if err != nil {
			return planner.OptimizeResult{}, fmt.Errorf("build directions for route rank %d: %w", selected[index].Rank, err)
		}
		selected[index].DirectionsURL = url
	}
	result.Routes = selected
	return result, nil
}

func buildService(cfg config.Config, nominatimBaseURL, osrmBaseURL string) planner.Service {
	return planner.Service{
		Solver: optimizer.NewSolver(cfg.Planner.MaxStops, cfg.Planner.MaxTopK),
		Geocoder: geocode.NewNominatim(nominatimBaseURL, cfg.HTTP.UserAgent,
			time.Duration(cfg.HTTP.GeocodeTimeoutSec)*time.Second),
		MatrixProvider: matrix.NewOSRM(osrmBaseURL, cfg.HTTP.UserAgent,
			time.Duration(cfg.HTTP.MatrixTimeoutSec)*time.Second),
		DefaultTopK: cfg.Planner.DefaultTopK,
	}
}

func printResult(result planner.OptimizeResult, durationUnit string) {
	fmt.Printf("\nTop %d route(s)\n", len(result.Routes))
	for _, route := range result.Routes {
		duration := route.DurationSeconds
		unit := "secs"
		if durationUnit == "minutes" {
			duration /= 60
			unit = "mins"
		}
		fmt.Printf("\n#%d  %.2f %s\n", route.Rank, duration, unit)
		for index, stop := range route.OrderedStops {
			fmt.Printf("  %d. %s\n", index+1, stop.Name)
		}
		if route.DirectionsURL != "" {
			fmt.Printf("  %s\n", route.DirectionsURL)
		}
	}
	fmt.Println()
}

func loadConfig(path string) (config.Config, string, error) {
	selected := path
	if path == "config.yaml" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			selected = "config.example.yaml"
		}
	}
	cfg, err := config.Load(selected)
	if err != nil {
		return config.Config{}, selected, err
	}
	return cfg, selected, nil
}

func firstNonEmpty(first, fallback string) string {
	if first != "" {
		return first
	}
	return fallback
}
