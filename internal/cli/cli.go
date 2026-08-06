package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"route-optimizer-go/internal/route"
	"route-optimizer-go/pepsi"
)

// Execute parses os.Args and runs the selected command.
func Execute() error {
	return RunArgs(os.Args[1:])
}

// RunArgs handles top-level help and dispatches an explicit command.
func RunArgs(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "-h", "--help":
		if len(args) != 1 {
			return fmt.Errorf("%s does not accept additional arguments", args[0])
		}
		printUsage(os.Stdout)
		return nil
	case "help":
		return runHelp(args[1:])
	}

	if !isCommand(args[0]) {
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
	return Run(args[0], args[1:])
}

func isCommand(value string) bool {
	switch value {
	case "all", "geocode", "matrix", "edge-metadata", "match-edges", "itinerary":
		return true
	default:
		return false
	}
}

func runHelp(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("help accepts at most one command, got %d", len(args))
	}
	if !isCommand(args[0]) {
		printUsage(os.Stderr)
		return fmt.Errorf("unknown help topic %q", args[0])
	}
	return Run(args[0], []string{"--help"})
}

// Usage prints top-level CLI help to stderr for callers reporting an error.
func Usage() {
	printUsage(os.Stderr)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  route-optimizer all [flags] [addresses-file]
  route-optimizer <command> [flags]
  route-optimizer help [command]

Run with no arguments to show this help.

Commands:
  all            Run geocode → matrix → itinerary
  geocode        Resolve addresses into stops
  matrix         Build the baseline OSRM duration matrix
  edge-metadata  Build directed OSRM route geometry
  match-edges    Match route geometry to NYC DOT link IDs
  itinerary      Solve top-K round trips and print Maps links

Examples:
  route-optimizer all examples/addresses.txt
  route-optimizer all -config config.yaml -addresses addresses.txt
  route-optimizer geocode -addresses addresses.txt -out data/stops.json
  route-optimizer matrix -stops data/stops.json -out data/matrix.json
  route-optimizer edge-metadata -stops data/stops.json -out data/edge_metadata.json
  route-optimizer match-edges -edge-metadata data/edge_metadata.json -out data/edge_metadata_matched.json
  route-optimizer itinerary -stops data/stops.json -matrix data/matrix.json
  route-optimizer itinerary -stops data/stops.json -matrix data/matrix.json -edge-state-fixture pepsi/testdata/edge_state_fixture.json
  route-optimizer itinerary -stops data/stops.json -matrix data/matrix.json -edge-metadata data/edge_metadata.json -dot-traffic
  route-optimizer itinerary -stops data/stops.json -refresh-matrix

Help:
  route-optimizer help
  route-optimizer help itinerary
  route-optimizer itinerary --help
`)
}

func newCommandFlagSet(name, invocation, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n  route-optimizer %s\n\n%s\n\nFlags:\n", invocation, summary)
		fs.PrintDefaults()
	}
	return fs
}

func parseCommandFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fs.SetOutput(os.Stdout)
		fs.Usage()
		return flag.ErrHelp
	}
	if err != nil {
		return fmt.Errorf("parse %s flags: %w (run 'route-optimizer help %s')", fs.Name(), err, fs.Name())
	}
	return nil
}

func rejectPositionalArgs(fs *flag.FlagSet) error {
	if fs.NArg() == 0 {
		return nil
	}
	return fmt.Errorf("%s does not accept positional arguments %q; use flags instead (run 'route-optimizer help %s')",
		fs.Name(), fs.Args(), fs.Name())
}

// runAll executes the stable, dependency-light demo path. The traffic-aware
// edge metadata and DOT matching stages remain explicit because they require
// substantially more live requests and optional Socrata configuration.
func runAll(args []string) error {
	fs := newCommandFlagSet("all", "all [flags] [addresses-file]", "Run geocode, matrix, and itinerary in sequence.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	addressesPath := fs.String("addresses", "", "newline-separated addresses file")
	stopsPath := fs.String("stops-out", "data/stops.json", "output stops JSON")
	matrixPath := fs.String("matrix-out", "data/matrix.json", "output duration matrix JSON")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}

	positional := fs.Args()
	if len(positional) > 1 {
		return fmt.Errorf("expected at most one addresses file, got %d", len(positional))
	}
	if len(positional) == 1 {
		if *addressesPath != "" {
			return fmt.Errorf("addresses file specified both positionally and with -addresses")
		}
		*addressesPath = positional[0]
	}
	selectedConfigPath := allConfigPath(*configPath)

	fmt.Printf("all: geocode → matrix → itinerary\n")
	if selectedConfigPath != *configPath {
		fmt.Printf("all: %s not found; using %s\n", *configPath, selectedConfigPath)
	}

	geocodeArgs := []string{
		"-config", selectedConfigPath,
		"-out", *stopsPath,
	}
	if *addressesPath != "" {
		geocodeArgs = append(geocodeArgs, "-addresses", *addressesPath)
	}
	if err := runGeocode(geocodeArgs); err != nil {
		return fmt.Errorf("all geocode: %w", err)
	}
	if err := runMatrix([]string{
		"-config", selectedConfigPath,
		"-stops", *stopsPath,
		"-out", *matrixPath,
	}); err != nil {
		return fmt.Errorf("all matrix: %w", err)
	}
	if err := runItinerary([]string{
		"-config", selectedConfigPath,
		"-stops", *stopsPath,
		"-matrix", *matrixPath,
	}); err != nil {
		return fmt.Errorf("all itinerary: %w", err)
	}

	fmt.Printf("all: complete; artifacts written to %s and %s\n", *stopsPath, *matrixPath)
	return nil
}

func allConfigPath(path string) string {
	if path != "config.yaml" {
		return path
	}
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return path
	}
	const examplePath = "config.example.yaml"
	if _, err := os.Stat(examplePath); err == nil {
		return examplePath
	}
	return path
}

// Run dispatches a subcommand with its flag args.
func Run(command string, args []string) error {
	var err error
	switch command {
	case "all":
		err = runAll(args)
	case "geocode":
		err = runGeocode(args)
	case "matrix":
		err = runMatrix(args)
	case "edge-metadata":
		err = runEdgeMetadata(args)
	case "match-edges":
		err = runMatchEdges(args)
	case "itinerary":
		err = runItinerary(args)
	default:
		Usage()
		return fmt.Errorf("unknown command %q", command)
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func runEdgeMetadata(args []string) error {
	fs := newCommandFlagSet("edge-metadata", "edge-metadata [flags]", "Build directed OSRM route geometry metadata for a stops artifact.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	outPath := fs.String("out", "data/edge_metadata.json", "output edge metadata JSON")
	osrmBaseURL := fs.String("osrm-base-url", pepsi.DefaultOSRMRouteBaseURL, "OSRM route service base URL")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return err
	}

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

func runMatchEdges(args []string) error {
	fs := newCommandFlagSet("match-edges", "match-edges [flags]", "Match OSRM edge geometry to NYC DOT traffic link IDs.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	edgeMetadataPath := fs.String("edge-metadata", "data/edge_metadata.json", "edge metadata JSON from edge-metadata")
	outPath := fs.String("out", "data/edge_metadata_matched.json", "output enriched edge metadata JSON")
	dotFixturePath := fs.String("dot-fixture", "", "fixture JSON array of DOT traffic records; if empty, fetch live DOT rows")
	dotEndpoint := fs.String("dot-endpoint", pepsi.DefaultDOTTrafficEndpoint, "NYC DOT Traffic Speeds Socrata endpoint")
	dotAppToken := fs.String("dot-app-token", os.Getenv("SOCRATA_APP_TOKEN"), "Socrata app token for DOT traffic requests")
	dotPageLimit := fs.Int("dot-page-limit", pepsi.DefaultDOTPageLimit, "DOT rows per Socrata page when fetching live rows")
	dotMaxPages := fs.Int("dot-max-pages", 0, "maximum DOT pages to fetch; 0 means until a short page")
	matchMaxDistance := fs.Float64("match-max-distance-m", pepsi.DefaultMatchMaxDistanceM, "maximum distance from any DOT link point to an OSRM edge")
	matchMaxAverageDistance := fs.Float64("match-max-average-distance-m", pepsi.DefaultMatchMaxAverageDistanceM, "maximum average distance from DOT link points to an OSRM edge")
	matchMaxLinksPerEdge := fs.Int("match-max-links-per-edge", 0, "maximum matched DOT links per edge; 0 means no limit")
	preserveExisting := fs.Bool("preserve-existing", false, "preserve existing matched_dot_link_ids and append new matches")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return err
	}

	cfg, err := route.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	cfg.ApplyRuntime()

	metadata, err := pepsi.ReadEdgeMetadata(*edgeMetadataPath)
	if err != nil {
		return fmt.Errorf("read edge metadata: %w", err)
	}

	var records []pepsi.DOTTrafficRecord
	sourceLabel := ""
	if *dotFixturePath != "" {
		if err := route.ReadJSON(*dotFixturePath, &records); err != nil {
			return fmt.Errorf("read DOT fixture %s: %w", *dotFixturePath, err)
		}
		sourceLabel = fmt.Sprintf("fixture %s", *dotFixturePath)
	} else {
		dotClient := pepsi.NewDOTClient(*dotEndpoint)
		dotClient.AppToken = *dotAppToken
		dotClient.UserAgent = cfg.HTTP.UserAgent
		dotClient.HTTPClient = &http.Client{
			Timeout: time.Duration(cfg.HTTP.OSRMTimeoutSec) * time.Second,
		}

		records, err = dotClient.FetchAllTrafficRecords(context.Background(), pepsi.DOTFetchAllOptions{
			Limit:    *dotPageLimit,
			MaxPages: *dotMaxPages,
		})
		if err != nil {
			return fmt.Errorf("fetch DOT traffic records: %w", err)
		}
		sourceLabel = fmt.Sprintf("DOT traffic %s", *dotEndpoint)
	}

	matched, summary, err := pepsi.MatchDOTLinks(metadata, records, pepsi.MatchOptions{
		MaxDistanceM:        *matchMaxDistance,
		MaxAverageDistanceM: *matchMaxAverageDistance,
		MaxLinksPerEdge:     *matchMaxLinksPerEdge,
		PreserveExisting:    *preserveExisting,
	})
	if err != nil {
		return fmt.Errorf("match DOT links: %w", err)
	}
	if err := pepsi.WriteEdgeMetadata(*outPath, matched); err != nil {
		return fmt.Errorf("write matched edge metadata %s: %w", *outPath, err)
	}

	fmt.Printf("match-edges: matched %d/%d edges with %d DOT link bindings from %s (%d candidate links, %d skipped links)\n",
		summary.MatchedEdgeCount, summary.EdgeCount, summary.TotalMatchedLinks, sourceLabel, summary.CandidateLinkCount, summary.SkippedLinkCount)
	fmt.Printf("match-edges: wrote %s\n", *outPath)
	return nil
}

func runItinerary(args []string) error {
	fs := newCommandFlagSet("itinerary", "itinerary [flags]", "Solve and print the top-K round trips, optionally with a traffic overlay.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	matrixPath := fs.String("matrix", "data/matrix.json", "duration matrix JSON from matrix")
	refreshMatrix := fs.Bool("refresh-matrix", false, "rebuild matrix.json from stops (uses OSRM/cache) before solving")
	edgeMetadataPath := fs.String("edge-metadata", "data/edge_metadata.json", "edge metadata JSON for traffic overlay")
	edgeStateFixturePath := fs.String("edge-state-fixture", "", "fixture JSON with per-edge current/previous traffic multipliers")
	dotTraffic := fs.Bool("dot-traffic", false, "apply live NYC DOT traffic using matched_dot_link_ids from edge metadata")
	dotEndpoint := fs.String("dot-endpoint", pepsi.DefaultDOTTrafficEndpoint, "NYC DOT Traffic Speeds Socrata endpoint")
	dotAppToken := fs.String("dot-app-token", os.Getenv("SOCRATA_APP_TOKEN"), "Socrata app token for DOT traffic requests")
	dotChunkSize := fs.Int("dot-chunk-size", pepsi.DefaultDOTChunkSize, "DOT link IDs per Socrata request")
	dotLimitPerLink := fs.Int("dot-limit-per-link", pepsi.DefaultDOTLimitPerLink, "DOT rows requested per link ID")
	trafficDefaultMultiplier := fs.Float64("traffic-default-multiplier", 1.0, "default traffic multiplier for edges without traffic state")
	trafficEMAAlpha := fs.Float64("traffic-ema-alpha", 0.3, "EMA alpha for current vs previous traffic multiplier")
	trafficMinMultiplier := fs.Float64("traffic-min-multiplier", 0.5, "minimum traffic multiplier clamp")
	trafficMaxMultiplier := fs.Float64("traffic-max-multiplier", 3.0, "maximum traffic multiplier clamp")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return err
	}

	if *edgeStateFixturePath != "" && *dotTraffic {
		return fmt.Errorf("choose either -edge-state-fixture or -dot-traffic, not both")
	}

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

	if *edgeStateFixturePath != "" || *dotTraffic {
		metadata, err := pepsi.ReadEdgeMetadata(*edgeMetadataPath)
		if err != nil {
			return fmt.Errorf("read edge metadata: %w", err)
		}
		opts := pepsi.TrafficOptions{
			DefaultMultiplier: *trafficDefaultMultiplier,
			EMAAlpha:          *trafficEMAAlpha,
			MinMultiplier:     *trafficMinMultiplier,
			MaxMultiplier:     *trafficMaxMultiplier,
		}

		var fetcher pepsi.EdgeStateFetcher
		sourceLabel := ""
		if *edgeStateFixturePath != "" {
			fixtureFetcher, err := pepsi.LoadFixtureEdgeStateFetcher(*edgeStateFixturePath)
			if err != nil {
				return fmt.Errorf("load edge state fixture: %w", err)
			}
			fetcher = fixtureFetcher
			sourceLabel = fmt.Sprintf("traffic fixture %s", *edgeStateFixturePath)
		}
		if *dotTraffic {
			dotClient := pepsi.NewDOTClient(*dotEndpoint)
			dotClient.AppToken = *dotAppToken
			dotClient.UserAgent = cfg.HTTP.UserAgent
			dotClient.HTTPClient = &http.Client{
				Timeout: time.Duration(cfg.HTTP.OSRMTimeoutSec) * time.Second,
			}
			dotClient.ChunkSize = *dotChunkSize
			dotClient.LimitPerLink = *dotLimitPerLink

			dotFetcher, err := pepsi.BuildDOTEdgeStateFetcher(context.Background(), dotClient, metadata, nil)
			if err != nil {
				return fmt.Errorf("build DOT edge state fetcher: %w", err)
			}
			fetcher = dotFetcher
			sourceLabel = fmt.Sprintf("DOT traffic %s", *dotEndpoint)
		}

		adjusted, snap, err := pepsi.ApplyEdgeTraffic(context.Background(), matrix, metadata, fetcher, opts)
		if err != nil {
			return fmt.Errorf("apply edge traffic: %w", err)
		}
		matrix = adjusted
		fmt.Printf("itinerary: applied %s via %s (%d edge multipliers, alpha %.2f)\n",
			sourceLabel, *edgeMetadataPath, len(snap.EdgeMultipliers), opts.EMAAlpha)
	}

	fmt.Printf("itinerary: solving top %d from %s + %s\n", cfg.Solver.TopK, *stopsPath, *matrixPath)
	routes, err := route.Solve(stops, matrix, cfg.Solver.TopK)
	if err != nil {
		return fmt.Errorf("solve: %w", err)
	}
	return route.PrintRoutes(routes, stops, cfg)
}

func runMatrix(args []string) error {
	fs := newCommandFlagSet("matrix", "matrix [flags]", "Build an OSRM baseline duration matrix for geocoded stops.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	stopsPath := fs.String("stops", "data/stops.json", "stops JSON from geocode")
	outPath := fs.String("out", "data/matrix.json", "output duration matrix JSON")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return err
	}

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
	fs := newCommandFlagSet("geocode", "geocode [flags]", "Resolve an addresses file into an ordered stops artifact.")
	configPath := fs.String("config", "config.yaml", "path to YAML config")
	addressesPath := fs.String("addresses", "", "newline-separated addresses file (defaults to config input)")
	outPath := fs.String("out", "data/stops.json", "output stops file")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return err
	}

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

	addressSource := *addressesPath
	if addressSource == "" {
		addressSource = cfg.Input.AddressesFile
	}
	if addressSource == "" {
		addressSource = "config input.addresses"
	}
	fmt.Printf("geocode: resolving %d addresses from %s\n", len(addresses), addressSource)

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
