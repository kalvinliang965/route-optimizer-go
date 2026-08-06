# Manhattan Multi-Stop Route Optimizer

Go CLI proof-of-concept for ranking small Manhattan driving itineraries. The
current demo path takes an ordered stop list, builds an OSRM duration matrix,
solves the top-K stop orders, and prints Google Maps direction links.

The next architecture layer is traffic-aware routing: keep the OSRM baseline
matrix stable, build route geometry metadata for each matrix cell, match those
routes to NYC DOT traffic links, smooth live traffic with EMA, then solve using
an adjusted in-memory matrix.

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
EMA traffic updater        -> smooths noisy traffic multipliers
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
| Edge Geometry Service (`pepsi`) | Initial working | For each matrix cell `i -> j`, fetches OSRM `/route` geometry and step metadata | `data/edge_metadata.json` |
| DOT Traffic Fetcher | Initial working | Pulls NYC DOT Traffic Speeds rows from Socrata/SODA by matched `link_id` or paginated full feed | in-memory DOT traffic records; cache planned |
| Local Matching Service | Initial working | Compares OSRM route geometry to DOT `link_points` locally and binds DOT `link_id`s to matrix cells | enriched `edge_metadata.json` |
| Traffic Snapshot Builder | Initial working | Loops edge metadata through fixture or DOT edge-state fetchers, applies EMA, and builds `TrafficSnapshot` multipliers | in-memory `TrafficSnapshot` |
| Traffic Cache/EMA Persistence | Planned | Persists previous smoothed traffic values for future EMA updates | planned `data/traffic_cache.json` |
| Solver Service | Working | Brute-force stop order search from fixed depot index `0`, retaining top-K routes | ranked `RouteResult`s |
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
  -> top-K routes + Google Maps links
```

### Target Traffic-Aware Flow

```text
data/stops.json
  -> Baseline Matrix Service
  -> data/matrix.json

data/stops.json
  -> Edge Geometry Service / pepsi
  -> data/edge_metadata.json

NYC DOT Socrata API
  -> DOT Traffic Fetcher
  -> in-memory DOT traffic records
  -> planned data/dot_traffic_cache.json

data/edge_metadata.json + DOT traffic rows/cache
  -> Local Matching Service
  -> matrix edge -> []DOT link_id bindings

DOT latest speeds + previous traffic_cache.json
  -> EMA Matrix Updater
  -> TrafficSnapshot multipliers

data/matrix.json + TrafficSnapshot
  -> ApplyTraffic
  -> adjusted in-memory matrix
  -> Solver Service
```

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

The local matcher decides which DOT links overlap each OSRM route cell. Once a
cell has matched links, the traffic updater can convert live speeds into a
multiplier:

```text
adjusted[i][j] = baseline[i][j] * traffic_multiplier[i][j]
```

Use EMA to smooth noisy live updates:

```text
ema = alpha * latest_multiplier + (1 - alpha) * previous_ema
```

Start with `alpha = 0.3` for demos. Later we can use a time-aware EMA based on
poll interval or half-life.

## Geometry Matching Strategy

Do not call Socrata once per OSRM intermediate point. That would be slow and
fragile. Instead:

1. Fetch DOT rows in batches and cache them.
2. Parse DOT `link_points` locally.
3. Fetch OSRM `/route` geometry once per matrix cell and cache it.
4. Simplify or sample OSRM points before matching.
5. Match OSRM route segments to nearby DOT link segments within a distance
   tolerance.
6. Store matched `link_id`s on the edge metadata.

If an edge has no DOT match, keep multiplier `1.0`.

## CLI

The current CLI uses stdlib `flag` with manual subcommand dispatch.

```bash
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

# 4. Solve top-K routes and print Maps links
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
config.example.yaml           # config template
```

Local demo artifacts may exist but are ignored by git:

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
- YAML traffic fixture loader.

WIP/planned:

- Match tuning/debug output for explaining why links did or did not match.
- EMA-backed traffic cache.
- Packaging matrix and solver boundaries.

Known dev note:

- The `pepsi` package is compile-safe and can build `edge_metadata.json`.
  DOT traffic can now be fetched and matched locally, but the matching thresholds
  are still demo-grade and should be inspected on real routes.

## Testing

Core route optimizer packages:

```bash
go test ./cmd/route-optimizer ./internal/...
```

Full repo, once `pepsi` is compile-safe:

```bash
go test ./...
```

## Demo Scope

This is built for small daily route batches, typically around 6-10 stops. The
solver uses exhaustive permutation search from a fixed depot, so it is a good
demo/pilot fit but not a large fleet VRP engine.
