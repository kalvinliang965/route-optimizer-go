# Route-Order Calculator

A modular Go calculator that ranks the top-K round trips for a small ordered
set of sites. Stop `0` is the depot, so every result begins and ends there.
The default `top_k` is `5`, and callers may choose another value within the
configured limit.

The calculator chooses waypoint order from a directed duration matrix. Google
Maps is only the executor: it receives the chosen order and handles navigation
between consecutive stops.

## Quick Start

The first run needs internet access for Nominatim geocoding and the OSRM
duration matrix. Later runs can reuse the provider cache under `data/cache`.

### Run the web app and HTTP API

Start the Go server from the repository root:

```bash
go run ./cmd/route-server -addr :8080 -config config.example.yaml
```

Keep that terminal running, then open <http://localhost:8080> in a browser. The
same process serves both the one-page frontend and the `/v1` JSON API. Confirm
the API from another terminal:

```bash
curl -i http://localhost:8080/healthz
curl -sS http://localhost:8080/v1/config
```

Stop the server with `Ctrl+C`. On Replit, the committed `.replit` file makes the
**Run** button start this same server on port `8080`. After clicking **Run**,
open **Preview**, then use its **New tab** button to open the `.replit.dev` URL.
You can also start it manually from the Replit Shell:

```bash
go run ./cmd/route-server -addr :8080 -config config.example.yaml
```

See [HTTP Server](#http-server) for complete `curl` examples.

### Run the complete CLI workflow

The `all` command is the one-command demo. It reads one address per line,
geocodes every address, requests one OSRM matrix, calculates the top routes,
and prints Google Maps links:

```bash
go run ./cmd/route-cli all \
  -config config.example.yaml \
  -addresses examples/addresses.txt \
  -top-k 5
```

The first nonblank line in the address file is always the depot—the start and
finish of every route. Blank lines are ignored. For example:

```text
Grand Central Terminal, New York, NY
Times Square, New York, NY
New York City Hall, New York, NY
```

In addition to printing the ranked routes, `all` writes these reusable files:

```text
data/stops.json
data/matrix.json
data/optimization.json
```

The address file may alternatively be positional. Put every flag before the
filename when using this form:

```bash
go run ./cmd/route-cli all -config config.example.yaml -top-k 5 examples/addresses.txt
```

To see every available command or only the `all` flags:

```bash
go run ./cmd/route-cli --help
go run ./cmd/route-cli help all
```

## Architecture

```text
CLI or browser/API handler
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
| `internal/geocode` | Nominatim adapter with validation, caching, and public-service pacing |
| `internal/matrix` | OSRM implementation of `planner.MatrixProvider` |
| `internal/maps` | Google Maps execution-link builder |
| `internal/config` | YAML process configuration |
| `internal/cache` | Persistent, expiring provider cache entries |
| `internal/app` | Shared CLI/server dependency construction |
| `internal/storage` | Atomic local JSON persistence for artifacts and cache entries |
| `internal/cli` | Thin command-line adapter over `planner.Service` |
| `internal/httpapi` | Root HTTP server, health route, and versioned API mounting |
| `internal/httpapi/v1` | Version 1 routes and handlers |
| `frontend` | Embedded one-page HTML, CSS, and vanilla JavaScript UI |
| `cmd/route-cli` | CLI executable |
| `cmd/route-server` | HTTP server process entry point |
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
- the same `planner.Service` is ready to be called by the versioned HTTP API.

The HTTP server now starts from `cmd/route-server`, and `GET /healthz` is
implemented. The root `internal/httpapi.Server` mounts a separate
`internal/httpapi/v1.Server` under `/v1`. `GET /v1/config` publishes the
configured planner limits. `POST /v1/optimize` accepts resolved stops and
`top_k`; the planner obtains an OSRM matrix through the shared cached provider
and returns ranked routes as JSON. `POST /v1/geocode` resolves a list of address
rows and returns a success or error for every row so a frontend can highlight
only the failures. Both CLI and HTTP use the same cached providers.
The API does not write complete request/result artifacts, but its provider
caches do persist resolved names, coordinates, and matrices as private local
files. The root page is a working dependency-free frontend. There is no
database, authentication, background worker, job queue, or cache-size quota.

The traffic experiment is preserved separately, but it is not part of the
current calculator or server milestone.

## Frontend

The root `frontend/` package contains one page with no npm packages or build
step:

```text
frontend/
  embed.go
  index.html
  styles.css
  app.js
```

The Go server embeds these files and serves them at `/`, so browser API calls
remain same-origin and require no CORS configuration. The page keeps the whole
workflow visible at once:

1. Choose **Address** or **Coordinates** on each row. The first row is the
   depot, and labels/notes remain optional.
2. Resolve the batch. Only Address rows are sent to `POST /v1/geocode`; row
   errors remain inline.
3. Each successful address automatically becomes a Coordinates row populated
   with the provider's real latitude and longitude. Those values can be edited
   directly, or the row can be changed back to Address and resolved again.
4. Choose `top_k` and call `POST /v1/optimize` after every row is ready.
5. Review ranked round trips and open one in Google Maps.

The page reads `GET /v1/config` on startup, so its stop count and `top_k`
controls follow the active YAML configuration rather than assuming defaults.

The built-in NYC demo uses one depot and three delivery stops so its directions
links fit Google Maps' documented mobile-browser limit of three intermediate
waypoints. Larger calculations still work, but mobile Maps links may omit extra
waypoints; the page displays this constraint next to the Maps notice.

Run it and open `http://localhost:8080`:

```bash
go run ./cmd/route-server -addr :8080 -config config.example.yaml
```

The committed `.replit` configuration makes Replit's **Run** button start the
server and maps its local port `8080` to external HTTP port `80`. Open the
automatically created **Preview**, then select **New tab** to use the full
`.replit.dev` site. The server also honors Replit's `PORT` environment variable
whenever `-addr` is not specified manually. Use an Autoscale or Reserved VM
deployment because the application requires its Go API, rather than a Static
deployment. Replit documents that a published app's filesystem is not
persistent, so the disk cache is a warm-run optimization there and may reset on
republish. See
[Replit deployment troubleshooting](https://docs.replit.com/build/troubleshooting).

## Provider Caches

CLI and server geocoding and OSRM requests use the same persistent cache
behavior. Successful geocodes default to 90 days, while complete matrices
default to 30 days. The longer geocode TTL is appropriate for fixed delivery
addresses. Both are configurable in `config.example.yaml` (start the server
with `-config config.example.yaml` to load that file):

```yaml
cache:
  enabled: true
  directory: data/cache
  geocode_ttl_hours: 2160
  matrix_ttl_hours: 720
```

Every entry is an atomic, versioned JSON file:

```text
data/cache/geocode/<sha256>.json
data/cache/matrices/<sha256>.json
```

Geocode keys include the Nominatim endpoint/query contract and a normalized
address. Case and repeated whitespace do not cause duplicate calls. Only
successful results are cached; invalid addresses and provider errors are not.
Coordinates entered directly bypass Nominatim and the geocode cache entirely.
They do not require an address; the optional label or generated Depot/Stop name
identifies them. Coordinates are still sent to the configured routing provider.

Matrix keys include the OSRM endpoint/profile and ordered stop coordinates at
the precision sent to OSRM. Stop names, IDs, and `top_k` do not affect the key.
Reordering or changing any coordinate causes a cache miss. Directly entered or
edited coordinates therefore use the matrix cache normally.

Concurrent same-key requests inside one process are combined so only one call
reaches the provider. Cache read/write failures are warnings and fall back to
the provider. Once an entry expires, the provider is tried first; if it is
unavailable, the expired entry is used so a previously warmed demo still runs.
Cache JSON files are written with owner-only (`0600`) permissions.

The old `data/geocode_cache.json` and `data/matrix_cache.json` formats are
legacy and are not read by active code because they lack provider/schema/expiry
metadata. Cache files are derived data and may be deleted safely; subsequent
requests rebuild them.

The default public Nominatim adapter is process-wide rate limited and serialized
to at most one provider request per second; cache hits do not wait. The UI must
submit geocoding only as an explicit action, not as type-ahead autocomplete,
and must display OpenStreetMap attribution. Do not send confidential or personal
addresses to the public service. Review the
[official Nominatim usage policy](https://operations.osmfoundation.org/policies/nominatim/)
before publishing the server; a production deployment should normally use a
self-hosted or contracted provider and add authentication plus storage quotas.

The default public OSRM table endpoint uses HTTPS. Matrix cache misses and
expired-entry refreshes share a process-wide serialized limiter that starts at
most one public OSRM request per second. A complete matrix needs one table
request, concurrent misses for the same matrix are still combined, and fresh
cache hits do not wait. Custom or self-hosted OSRM endpoints are not paced by
this public-demo limiter. Review the
[OSRM demo server policy](https://github.com/Project-OSRM/osrm-backend/wiki/Demo-server)
before publishing against the shared endpoint.

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

The CLI requires an explicit command. Running it without a command prints help;
use `all` for the complete user-facing workflow. The CLI automatically falls
back to `config.example.yaml` when `config.yaml` is absent, though explicitly
selecting the example configuration makes demos reproducible.

```bash
# Complete address -> matrix -> top-K workflow
go run ./cmd/route-cli all \
  -config config.example.yaml \
  -addresses examples/addresses.txt \
  -top-k 5

# Optional: run the same work as separate, inspectable stages
go run ./cmd/route-cli geocode \
  -config config.example.yaml \
  -addresses examples/addresses.txt \
  -out data/stops.json

go run ./cmd/route-cli matrix \
  -config config.example.yaml \
  -stops data/stops.json \
  -out data/matrix.json

# Recalculate from existing JSON artifacts without external requests
go run ./cmd/route-cli optimize \
  -config config.example.yaml \
  -stops data/stops.json \
  -matrix data/matrix.json \
  -out data/optimization.json \
  -top-k 5

# Present the best route as a Google Maps itinerary
go run ./cmd/route-cli itinerary \
  -config config.example.yaml \
  -plan data/optimization.json \
  -rank 1

# Use -rank 0 to present every ranked route
go run ./cmd/route-cli itinerary -plan data/optimization.json -rank 0

# Help
go run ./cmd/route-cli --help
go run ./cmd/route-cli help all
go run ./cmd/route-cli help optimize
```

`all` accepts either `-addresses FILE` or one positional address file, but not
both. Its output paths can be changed with `-stops-out`, `-matrix-out`, and
`-plan-out`. Run `go run ./cmd/route-cli help all` for the complete flag list.
The staged commands are mainly useful for debugging or replaying saved data;
they are not required for a normal demo.

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

Internally, `planner.Service` accepts either:

- resolved `Stops`, plus an optional caller-supplied duration matrix; or
- `Addresses`, allowing the planner to use its geocoder and matrix provider.

If no matrix is supplied, the configured provider builds one. The public
`POST /v1/optimize` endpoint deliberately accepts only resolved stops and
`top_k`, keeping the matrix as a backend implementation detail.

Geocode request:

```json
{
  "addresses": [
    "Times Square, New York, NY",
    "an invalid address"
  ]
}
```

`POST /v1/geocode` returns HTTP `200` for a processed batch even when individual
rows fail. Each result retains its zero-based input index:

```json
{
  "results": [
    {
      "index": 0,
      "address": "Times Square, New York, NY",
      "stop": {
        "id": "stop-0",
        "name": "Times Square, New York",
        "lon": -73.986,
        "lat": 40.757
      }
    },
    {
      "index": 1,
      "address": "an invalid address",
      "error": "no geocode result"
    }
  ]
}
```

Malformed JSON and unknown fields return `400`. Missing or oversized address
batches return `422`. Blank strings are row-level errors so the frontend can
highlight their exact positions.

Suggested HTTP request:

```json
{
  "stops": [
    {"id": "depot", "name": "Warehouse", "lat": 40.75, "lon": -73.98},
    {"id": "a", "name": "Customer A", "lat": 40.72, "lon": -74.00}
  ],
  "top_k": 5
}
```

Suggested response fields already exist on `planner.OptimizeResult`:

```json
{
  "matrix_source": "provider",
  "top_k": 5,
  "routes": [
    {
      "rank": 1,
      "path": [0, 1, 0],
      "duration_seconds": 660,
      "directions_url": "https://www.google.com/maps/dir/?api=1..."
    }
  ]
}
```

## HTTP Server

Start the HTTP server locally and leave it running:

```bash
go run ./cmd/route-server -addr :8080 -config config.example.yaml
```

It serves the frontend at <http://localhost:8080>. The following requests can
be run from a second terminal.

Check the process and inspect its active limits:

```bash
curl -i http://localhost:8080/healthz
curl -sS http://localhost:8080/v1/config
```

Resolve address rows. A processed batch returns HTTP `200` even when an
individual row contains an `error`:

```bash
curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "addresses": [
      "Times Square, New York, NY",
      "New York City Hall, New York, NY"
    ]
  }' \
  http://localhost:8080/v1/geocode
```

Calculate routes from resolved or manually supplied coordinates. The first
stop is the depot and is automatically placed at both ends of every result:

```bash
curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "stops": [
      {"id":"depot","name":"Warehouse","lat":40.7500,"lon":-73.9800},
      {"id":"a","name":"Customer A","lat":40.7200,"lon":-74.0000}
    ],
    "top_k": 5
  }' \
  http://localhost:8080/v1/optimize
```

`POST /v1/geocode` and `POST /v1/optimize` are separate so clients can review
or correct coordinates between the two operations. The browser UI performs
this sequence for you.

Current route status:

```text
GET  /               embedded one-page frontend
GET  /healthz       process health
GET  /v1/config     planner defaults and request limits
POST /v1/geocode    resolve address rows with per-row results
POST /v1/optimize   build a cached matrix and rank resolved stops
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
