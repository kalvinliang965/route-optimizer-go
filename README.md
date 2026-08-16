# Route-Order Calculator

A modular Go calculator that ranks the top-K round trips for a small ordered
set of sites. Stop `0` is the depot, so every result begins and ends there.
The default `top_k` is `5`, and callers may choose another value within the
configured limit.

The calculator chooses waypoint order from a directed duration matrix. Google
Maps is only the executor: it receives the chosen order and handles navigation
between consecutive stops.

## Architecture

```text
CLI or future HTTP handler
          |
          v
    planner.Service
      |         |
      v         v
 optimizer   external ports
  (pure)     |     |      |
             v     v      v
        Nominatim OSRM Google Maps link
```

| Package | Responsibility |
|---|---|
| `internal/optimizer` | Pure models, matrix validation, and streaming top-K round-trip solver |
| `internal/planner` | Application workflow and interfaces for external dependencies |
| `internal/geocode` | Nominatim implementation of `planner.Geocoder` |
| `internal/matrix` | OSRM implementation of `planner.MatrixProvider` |
| `internal/maps` | Google Maps execution-link builder |
| `internal/config` | YAML process configuration |
| `internal/storage` | CLI-only address and JSON artifact persistence |
| `internal/cli` | Thin command-line adapter over `planner.Service` |
| `internal/httpapi` | Reserved transport boundary for the server exercise |
| `cmd/route-cli` | CLI executable |
| `cmd/route-server` | Reserved server executable directory |
| `internal/deprecated/traffic` | Preserved WIP OSRM geometry, NYC DOT matching, and EMA overlay experiment |

Dependency rule: `optimizer` imports no application, transport, filesystem, or
HTTP package. `planner` owns the use case and depends on small interfaces.
Adapters implement those interfaces. A CLI or HTTP handler calls the same
`planner.Service`.

## Current State

The project is currently a working, demonstrable route-order calculator:

- `route-cli all` accepts an address file, geocodes it, builds an OSRM duration
  matrix, calculates the top-K round trips, and prints Google Maps links;
- `route-cli optimize` calculates routes from existing stops and matrix JSON,
  then writes a reusable optimization result without making provider requests;
- `route-cli itinerary` reads that result, selects a rank, and builds its Google
  Maps URL without rerunning the solver;
- the depot is always stop `0`, and every calculated path returns to stop `0`;
- `top_k` defaults to `5` and is configurable per request;
- the optimizer, planner, external adapters, storage, and CLI are separated and
  covered by tests; and
- the same `planner.Service` is ready to be called by an HTTP transport.

The HTTP server and frontend are intentionally not implemented yet. There is
also no database, authentication, background worker, or job queue. The next
active milestone is implementing `GET /healthz` and `POST /v1/optimize` in
`internal/httpapi`, then wiring them from `cmd/route-server`.

The traffic experiment is preserved separately, but it is not part of the
current calculator or server milestone.

## Preserved Traffic Experiment

The unique real-time traffic work has **not been deleted**. Its implementation,
fixtures, and tests are isolated under `internal/deprecated/traffic`.

That path is currently **stalled and not wired into `route-cli` or the future
HTTP server**. The active product is the top-K route-order calculator. This
keeps the server boundary small while preserving the traffic research for a
later iteration.

The former `internal/route` and legacy CLI copies were removed because they
duplicated the active solver, geocoder, matrix provider, storage, maps, and
configuration packages. The preserved traffic package now reuses
`optimizer.Stop` and `storage` instead of maintaining a second route domain.

The traffic experiment is paused because:

- the NYC DOT source is a large append-only historical feed, so scanning it as
  though it were a small current snapshot is too slow for a reliable demo;
- DOT sensors cover selected major roads rather than every route, so valid
  routes can produce no geometry matches;
- OSRM is used to estimate route geometry while Google Maps executes the final
  itinerary, so their chosen roads may differ; and
- reusable edge metadata still needs a stop/coordinate and provider fingerprint
  before cached matches can be trusted.

See `internal/deprecated/traffic/README.md` for the preserved pipeline and its
remaining work. When the experiment resumes, expose it behind a planner
interface instead of coupling it directly to HTTP handlers or rebuilding a
second CLI/application stack.

## CLI

The CLI automatically falls back to `config.example.yaml` when `config.yaml`
is absent.

```bash
# Complete address -> matrix -> top-K workflow
go run ./cmd/route-cli all -top-k 5 examples/addresses.txt

# Recalculate from existing JSON artifacts without external requests
go run ./cmd/route-cli optimize \
  -stops data/stops.json \
  -matrix data/matrix.json \
  -out data/optimization.json \
  -top-k 5

# Present the best route as a Google Maps itinerary
go run ./cmd/route-cli itinerary \
  -plan data/optimization.json \
  -rank 1

# Use -rank 0 to present every ranked route
go run ./cmd/route-cli itinerary -plan data/optimization.json -rank 0

# Help
go run ./cmd/route-cli --help
go run ./cmd/route-cli help optimize
```

`cmd/route-optimizer` remains as a compatibility alias while existing scripts
move to `cmd/route-cli`.

### How the CLI is written

The executable should contain almost no logic. It delegates to the CLI adapter
and converts a returned error into a process exit code:

```go
// cmd/route-cli/main.go
package main

import (
    "fmt"
    "os"

    "route-optimizer-go/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}
```

Inside `internal/cli`, a command should parse flags and files, call the planner,
and present the result. It should not calculate route permutations itself. A
small optimize command follows this pattern:

```go
func runOptimize(ctx context.Context, service planner.Service, args []string) error {
    flags := flag.NewFlagSet("optimize", flag.ContinueOnError)
    stopsPath := flags.String("stops", "data/stops.json", "stops JSON")
    matrixPath := flags.String("matrix", "data/matrix.json", "matrix JSON")
    outPath := flags.String("out", "data/optimization.json", "result JSON")
    topK := flags.Int("top-k", 5, "number of routes to return")
    if err := flags.Parse(args); err != nil {
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

    result, err := service.Optimize(ctx, planner.OptimizeRequest{
        Stops:                 stops,
        DurationMatrixSeconds: durations,
        TopK:                  *topK,
    })
    if err != nil {
        return err
    }

    return storage.WriteJSON(*outPath, result)
}
```

`itinerary` then reads that result, selects `-rank 1` by default, and asks the
Google Maps adapter to build a URL from the already-calculated path. It does not
load the matrix or call the solver again.

This separation also gives future traffic work a clear insertion point:

```text
baseline matrix
  -> optional traffic matrix adjustment
  -> optimize and write ranked result
  -> itinerary presentation
```

The command-dispatch flow is:

```text
cmd/route-cli/main.go
  -> cli.Execute
  -> cli.RunArgs
  -> command flag/file handling
  -> optimize: planner.Service.Optimize
  -> itinerary: Maps presentation of stored result
  -> formatted terminal output
```

See `internal/cli/cli.go` for the complete flag dispatch and dependency wiring.

## Planner API

The server handler should construct a `planner.OptimizeRequest` with either:

- resolved `Stops`, plus an optional caller-supplied duration matrix; or
- `Addresses`, allowing the planner to use its geocoder and matrix provider.

If no matrix is supplied, the configured provider builds one. `OptimizeResult`
contains ranked paths, ordered stops, durations, matrix source, and Maps URLs.

Suggested HTTP request:

```json
{
  "stops": [
    {"id": "depot", "name": "Warehouse", "lat": 40.75, "lon": -73.98},
    {"id": "a", "name": "Customer A", "lat": 40.72, "lon": -74.00}
  ],
  "top_k": 5,
  "duration_matrix_seconds": [[0, 300], [360, 0]]
}
```

Suggested response fields already exist on `planner.OptimizeResult`:

```json
{
  "matrix_source": "provided",
  "top_k": 5,
  "routes": [
    {
      "rank": 1,
      "path": [0, 1, 0],
      "duration_seconds": 660,
      "directions_url": "https://www.google.com/maps/dir/?..."
    }
  ]
}
```

## Server Exercise

Implement these first:

```text
GET  /healthz
POST /v1/optimize
```

The handler should only decode JSON, perform HTTP-level validation, call
`planner.Service.Optimize(r.Context(), request)`, map errors to status codes,
and encode JSON. Start with Go's `net/http`; a framework is unnecessary for
this API.

## Solver Scope

The solver still performs factorial brute-force search, but it no longer stores
all permutations. It evaluates each permutation immediately and retains only a
size-K heap, reducing memory from factorial growth to `O(stops + K)`.

This remains intended for small batches. `max_stops: 10` is a guard, not a
latency guarantee under concurrent server load.

## Testing

```bash
go test ./...
go vet ./...
```
