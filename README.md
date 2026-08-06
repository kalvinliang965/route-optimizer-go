# Manhattan Multi-Stop Route Optimizer

Go CLI proof-of-concept for ranking small Manhattan driving itineraries. The
current demo path takes an input stop list, builds an OSRM duration matrix,
solves the top-K round trips, and prints Google Maps direction links. Stop `0`
(the first input address) is the depot, so every result starts and ends there.

An optional traffic-aware path is also implemented at demo quality. It keeps the
OSRM baseline matrix stable, builds route geometry metadata for each matrix
cell, matches those routes to NYC DOT traffic links, derives congestion
multipliers, and solves against an adjusted in-memory matrix. Matching
inspection and persistent traffic history are still planned.

## Summary Contract

This project is a **planner**, not the final navigation executor. It optimizes
the stop order under our own planning model: OSRM baseline durations plus an
optional NYC DOT traffic overlay. Google Maps is the handoff for turn-by-turn
execution after a route order is chosen.

That means the top route is best under **our** matrix at solve time, not a claim
that Google Maps would choose the same roads or rank every stop order the same
way. Google may dynamically route around congestion between two stops. We keep
this honest by showing top-K options, keeping traffic multipliers conservative,
and treating Maps links as execution links rather than optimization inputs.

## Core Idea

Every route between two stops is one matrix cell:

```text
matrix[i][j] = baseline drive time from stop i to stop j
```

OSRM can tell us the baseline time. To make that cell traffic-aware, we also
need to know which road geometry the cell uses and which NYC DOT traffic links
overlap that geometry.

```text
OSRM baseline matrix       -> stable on disk
OSRM route geometry        -> edge metadata per matrix cell
NYC DOT traffic records    -> latest speeds for DOT link_ids
local matcher              -> binds DOT link_ids to matrix cells
optional EMA updater       -> smooths multipliers when previous state exists
solver                     -> ranks routes using adjusted matrix
```

Important rule: `data/matrix.json` remains the OSRM baseline. Traffic produces a
temporary adjusted matrix for solving; it should not overwrite the baseline.

## Logical Services

These are "services" as architecture boundaries. Today most live inside one Go
module and CLI; later we can package the matrix builder and solver separately.

| Service | Status | Owns | Output |
|---|---|---|---|
| Address/Geocode Service | Working | Reads address input and resolves lat/lon with Nominatim plus cache | `data/stops.json`, `data/geocode_cache.json` |
| Baseline Matrix Service | Working | Builds the full N x N OSRM duration table for the stops | `data/matrix.json`, `data/matrix_cache.json` |
| Edge Geometry Service (`pepsi`) | Working, demo-grade | For each matrix cell `i -> j`, fetches OSRM `/route` geometry and step metadata | `data/edge_metadata.json` |
| DOT Traffic Fetcher | Working, demo-grade | Pulls NYC DOT Traffic Speeds rows from Socrata/SODA by matched `link_id` or paginated full feed | in-memory DOT traffic records; cache planned |
| Local Matching Service | Working, tuning needed | Compares OSRM route geometry to DOT `link_points` locally and binds DOT `link_id`s to matrix cells | `data/edge_metadata_matched.json` |
| Traffic Snapshot Builder | Working, demo-grade | Converts fixture or live DOT edge state into optional EMA-smoothed `TrafficSnapshot` multipliers | in-memory `TrafficSnapshot` |
| Traffic Cache/EMA Persistence | Planned | Persists previous smoothed traffic values for future EMA updates | planned `data/traffic_cache.json` |
| Solver Service | Working | Brute-force round-trip search from fixed depot index `0`, retaining top-K tours | ranked `RouteResult`s |
| Maps/Itinerary Output | Working | Prints ranked stops, total duration, and Google Maps deep links | terminal output |

## Data Flow

### Current Demo Flow

```text
addresses.txt
  -> geocode
  -> data/stops.json
  -> matrix
  -> data/matrix.json
  -> itinerary
  -> top-K round trips (depot -> all stops -> depot)
  -> Google Maps links
```

Run that entire flow explicitly with the `all` command:

```bash
go run ./cmd/route-optimizer all examples/addresses.txt
```

This writes `data/stops.json` and `data/matrix.json`, then immediately prints
the ranked routes. It uses `config.yaml` when present and otherwise falls back
to the tracked `config.example.yaml`. The input can also be passed as a flag,
and artifact paths can be overridden when a clean demo directory is useful:

```bash
go run ./cmd/route-optimizer all \
  -config config.yaml \
  -addresses addresses.txt \
  -stops-out data/stops.json \
  -matrix-out data/matrix.json
```

The edge-metadata and live-DOT stages remain explicit advanced commands because
they require many more live requests and matching configuration.

### Optional Traffic-Aware Flow

```text
data/stops.json
  -> matrix
  -> data/matrix.json

data/stops.json
  -> edge-metadata (one OSRM /route request per directed stop pair)
  -> data/edge_metadata.json

data/edge_metadata.json + NYC DOT rows (live or fixture)
  -> match-edges
  -> data/edge_metadata_matched.json

data/matrix.json + data/edge_metadata_matched.json
  -> itinerary -dot-traffic
  -> fetch latest rows for matched DOT link_ids
  -> current TrafficSnapshot multipliers
  -> adjusted in-memory matrix
  -> top-K round trips + Google Maps links

Alternative deterministic demo:
data/matrix.json + data/edge_metadata.json + edge-state fixture
  -> itinerary -edge-state-fixture ...
  -> optional EMA using current/previous fixture values
  -> adjusted in-memory matrix
  -> top-K round trips + Google Maps links
```

The live CLI does not yet persist previous traffic multipliers between runs.
Consequently, a live run uses the current multiplier directly unless previous
state is supplied programmatically. Persistent DOT and EMA caches remain
planned.

## Traffic Model

DOT/Socrata does not return "stop 0 to stop 1 is slow." It returns traffic rows
for road links:

```text
link_id
speed
travel_time
data_as_of
link_points
link_name
borough
```

The local matcher decides which DOT links overlap each OSRM route cell. For a
matched edge, the live updater compares the edge's OSRM baseline speed with the
average observed DOT speed:

```text
baseline_mph = (edge_distance_m / edge_duration_sec) converted to mph
current_multiplier = baseline_mph / observed_dot_mph
```

It then applies the multiplier to a copy of the baseline matrix:

```text
adjusted[i][j] = baseline[i][j] * traffic_multiplier[i][j]
```

When previous state is available, EMA can smooth noisy updates:

```text
ema = alpha * latest_multiplier + (1 - alpha) * previous_ema
```

The default `alpha` is `0.3`. Fixture-backed demos can provide previous values;
live cross-run smoothing requires the planned persistent traffic cache.

## Geometry Matching Strategy

The current matching flow avoids calling Socrata once per OSRM geometry point:

1. `edge-metadata` fetches every directed OSRM route and writes a reusable
   artifact. It does not maintain a separate automatic geometry cache.
2. `match-edges` fetches DOT rows in pages, or reads a DOT fixture.
3. Parse DOT `link_points` locally.
4. Compare DOT link points with each OSRM route polyline using configurable
   maximum and average distance thresholds.
5. Store matched `link_id`s in a new edge metadata artifact.
6. During `itinerary -dot-traffic`, fetch only the latest rows for those matched
   IDs and build the in-memory traffic snapshot.

In short:

```text
OSRM geometry is persisted as an artifact and reused explicitly.
DOT rows and previous EMA state are not yet persisted automatically.
```

Earlier design ideas such as route-point sampling, spatial indexing, and
automatic geometry/DOT caches are future tuning work rather than current
behavior.

With the default settings, an edge with no usable DOT match keeps multiplier
`1.0`.

## CLI

The current CLI uses stdlib `flag` with explicit subcommand dispatch. Running
it without arguments prints top-level help; unknown commands and flags return
errors instead of being interpreted as input paths.

```bash
# All-stage demo: addresses -> stops -> matrix -> ranked round trips
go run ./cmd/route-optimizer all examples/addresses.txt

# Top-level and command-specific help
go run ./cmd/route-optimizer --help
go run ./cmd/route-optimizer help itinerary
go run ./cmd/route-optimizer itinerary --help

# Or run individual stages:

# 1. Addresses -> geocoded stops
go run ./cmd/route-optimizer geocode \
  -addresses addresses.txt \
  -out data/stops.json

# 2. Stops -> OSRM baseline matrix
go run ./cmd/route-optimizer matrix \
  -stops data/stops.json \
  -out data/matrix.json

# 3. Stops -> OSRM route geometry per matrix cell
go run ./cmd/route-optimizer edge-metadata \
  -stops data/stops.json \
  -out data/edge_metadata.json

# 4. Solve top-K round trips and print Maps links
go run ./cmd/route-optimizer itinerary \
  -stops data/stops.json \
  -matrix data/matrix.json

# 5. Enrich edge metadata with local DOT link matches
go run ./cmd/route-optimizer match-edges \
  -edge-metadata data/edge_metadata.json \
  -out data/edge_metadata_matched.json \
  -dot-app-token "$SOCRATA_APP_TOKEN"

# 6. Solve using fake/demo edge traffic fixture + EMA
go run ./cmd/route-optimizer itinerary \
  -stops data/stops.json \
  -matrix data/matrix.json \
  -edge-metadata data/edge_metadata.json \
  -edge-state-fixture pepsi/testdata/edge_state_fixture.json

# 7. Solve using live DOT traffic for matched edges
go run ./cmd/route-optimizer itinerary \
  -stops data/stops.json \
  -matrix data/matrix.json \
  -edge-metadata data/edge_metadata_matched.json \
  -dot-traffic \
  -dot-app-token "$SOCRATA_APP_TOKEN"

# Rebuild matrix before solving
go run ./cmd/route-optimizer itinerary \
  -stops data/stops.json \
  -refresh-matrix
```

Planned commands:

```bash
# Refresh DOT traffic and EMA cache
go run ./cmd/route-optimizer refresh-traffic \
  -stops data/stops.json \
  -edges data/edge_metadata.json
```

## Repository Layout

```text
cmd/route-optimizer/main.go   # CLI entrypoint
internal/cli/cli.go           # subcommands and orchestration
internal/route/               # geocode, matrix, solver, maps, traffic helpers
pepsi/                        # current scratch area for edge geometry / DOT ideas
examples/addresses.txt        # validated congestion-oriented demo input
config.example.yaml           # config template
```

Local runtime artifacts may exist outside the tracked source:

```text
addresses.txt
config.yaml
data/stops.json
data/matrix.json
data/geocode_cache.json
data/matrix_cache.json
data/traffic_fixture.yaml
docs/
```

## Current Implementation State

Working:

- Geocode command with Nominatim cache.
- Matrix command with OSRM table cache.
- Edge metadata command with OSRM `/route` geometry artifacts.
- Itinerary command with top-K solver and Google Maps links.
- `ApplyTraffic` multiplier engine.
- `pepsi` traffic snapshot loop from edge metadata to EMA-smoothed adjusted matrix.
- Fixture-backed edge-state fetcher for fake/demo traffic flows.
- Itinerary fixture traffic flags for solving against an adjusted in-memory matrix.
- DOT/Socrata edge-state fetcher for edges that already have `matched_dot_link_ids`.
- Local DOT matcher that fills `matched_dot_link_ids` from OSRM geometry and DOT
  `link_points`.
- Match-edges command for writing enriched edge metadata.
- Itinerary DOT traffic flags for solving against a live DOT-adjusted in-memory matrix.
- YAML traffic fixture loader (library-level; the CLI uses the JSON edge-state fixture).

WIP/planned:

- Match tuning/debug output for explaining why links did or did not match.
- Automatic edge geometry and DOT row caches.
- Persistent EMA traffic history for live runs.
- Packaging matrix and solver boundaries.
- Saving successful geocode entries when a later address in the batch fails.

Known dev note:

- The `pepsi` package is compile-safe and can build `edge_metadata.json`.
  DOT traffic can now be fetched and matched locally, but the matching thresholds
  are still demo-grade and should be inspected on real routes.

## Testing

The full repository is compile-safe and can be tested with:

```bash
go test ./...
```

## Demo Scope

This is built for small daily route batches, typically around 6-9 stops. The
solver materializes all `(n-1)!` permutations, fixes stop `0` as the depot, and
adds the depot again at the end of each tour. The configured `max_stops` is a
guard, not a performance promise: values near 15 are computationally
impractical with the current solver. This is a demo/pilot optimizer, not a
large-fleet VRP engine.
