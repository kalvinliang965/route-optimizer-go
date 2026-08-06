# Pepsi: Edge Geometry Metadata Builder

`pepsi` is the scratch/WIP package for turning a plain OSRM duration matrix into
something traffic-aware.

The core route optimizer already has this:

```text
matrix[i][j] = baseline drive time from stop i to stop j
```

`pepsi` should answer the missing question:

```text
What road geometry does matrix cell i -> j use?
```

Once we know that geometry, another layer can match it to NYC DOT traffic links.
Then `pepsi` can fetch live DOT speeds for those matched links, smooth traffic
with EMA, and update the in-memory matrix before solving.

## Accuracy Contract

`pepsi` does not produce the exact Google Maps route. It produces the OSRM route
geometry that our planner uses to estimate which matrix cells are affected by
NYC DOT traffic links.

The final executor is Google Maps. Google may choose different turn-by-turn
roads at navigation time, especially if it sees live conditions that our OSRM +
DOT model does not capture. So the honest claim is:

```text
The optimizer ranks stop orders under our OSRM + DOT planning model.
Google Maps executes the chosen stop order with its own routing model.
```

## Package Goal

For an ordered stop list, build an edge metadata artifact for every directed
stop pair:

```text
stop i -> stop j
```

Each edge should contain:

- `from_stop`: source stop index
- `to_stop`: destination stop index
- `baseline_duration_sec`: OSRM route duration for this edge
- `baseline_distance_m`: OSRM route distance for this edge
- `geometry`: sampled route coordinates from OSRM `/route`
- `steps`: optional OSRM step names and maneuver metadata
- `matched_dot_link_ids`: empty at first, filled later by local matching

Planned output:

```text
data/edge_metadata.json
```

## Not This Package

`pepsi` should stay focused. It should not own:

- the top-K route solver
- itinerary printing
- Google Maps links

Those belong to the main route package / CLI pipeline. The traffic snapshot
helpers currently live here because they are still tied to the edge metadata
experiment, but they produce data for the solver rather than solving routes.

`pepsi` may eventually include the local geometry matcher, but it should still
produce data rather than directly solve routes.

## Why Edge Metadata Exists

OSRM `/table` is efficient for building the baseline duration matrix, but it
only gives aggregate cell values:

```text
0 -> 1 = 620 seconds
0 -> 2 = 480 seconds
```

Traffic data from NYC DOT is link-based:

```text
link_id
speed
travel_time
data_as_of
link_points
link_name
borough
```

DOT does not tell us "stop 0 to stop 1 is slow." It tells us "road link 123 is
slow." Edge metadata bridges that gap:

```text
matrix cell 0 -> 1
  -> OSRM route geometry
  -> nearby/matched DOT link_ids
  -> traffic multiplier for this cell
```

## Target Flow

```text
data/stops.json
  -> pepsi edge builder
  -> OSRM /route request for each i -> j
  -> data/edge_metadata.json

data/edge_metadata.json + DOT traffic cache
  -> local matcher
  -> edge metadata with matched DOT link_ids

matched DOT link_ids + live speeds
  -> DOT edge-state fetcher
  -> traffic multiplier / EMA layer
  -> TrafficSnapshot

TrafficSnapshot + data/matrix.json
  -> ApplyTraffic
  -> adjusted in-memory matrix
  -> solver
```

## Artifact Sketch

```json
{
  "generated_at": "2026-08-05T00:00:00Z",
  "source": "osrm-route",
  "stops_hash": "optional-stable-hash",
  "edges": [
    {
      "from_stop": 0,
      "to_stop": 1,
      "baseline_duration_sec": 620.4,
      "baseline_distance_m": 3201.7,
      "geometry": [
        {"lat": 40.729661, "lon": -73.974688},
        {"lat": 40.731200, "lon": -73.976100}
      ],
      "steps": [
        {
          "name": "1st Avenue",
          "duration_sec": 120.0,
          "distance_m": 600.0
        }
      ],
      "matched_dot_link_ids": []
    }
  ]
}
```

## OSRM Route Fetching

Use OSRM `/route`, not `/table`, for this package:

```text
/route/v1/driving/{lon1},{lat1};{lon2},{lat2}?overview=full&steps=true&geometries=geojson
```

Important implementation notes:

- Fetch directed edges; `i -> j` and `j -> i` may differ.
- Skip diagonal edges where `i == j`.
- Reuse the same HTTP timeout and user-agent config style as the main package.
- Cache results on disk so demo runs do not repeatedly hit OSRM.
- Parse into typed structs, not `map[string]interface{}`.

## Local Matching

Do not query Socrata once per OSRM intermediate point. Instead:

1. Fetch DOT traffic rows separately, either from a fixture or the paginated
   Socrata feed.
2. Parse DOT `link_points` into local coordinate polylines.
3. Compare each DOT link polyline to nearby OSRM route geometry locally.
4. Keep links whose DOT points are within conservative distance thresholds.
5. Store matched `link_id`s back on the edge metadata.

For a demo, start conservative:

- sample route points every `50-150m`
- match within about `50-100m`
- optionally prefer DOT links whose `link_name` resembles an OSRM step name
- if no link matches an edge, leave `matched_dot_link_ids` empty

Empty matches are acceptable; the traffic layer should then use multiplier
`1.0` for that edge.

## Implementation Milestones

1. Make `pepsi` compile-safe so `go test ./...` can run. Done.
2. Replace the hard-coded probe with typed OSRM route response structs. Done.
3. Add `FetchEdge(ctx, stops, from, to)` with mocked OSRM tests. Done.
4. Add a recorded OSRM `testdata` fixture and pure parser test. Done.
5. Add `BuildEdgeMetadata(ctx, stops []route.Stop)`. Done.
6. Add JSON read/write helpers for `edge_metadata.json`. Done.
7. Add tests using `httptest` fixtures for full artifact generation. Done.
8. Add a CLI command, `edge-metadata`, that reads `data/stops.json` and writes
   `data/edge_metadata.json`. Done.
9. Add `BuildTrafficSnapshot` and `ApplyEdgeTraffic` around an abstract edge
   state fetcher, with EMA smoothing. Done.
10. Add fixture-backed edge-state fetcher for fake/demo traffic flows. Done.
11. Wire fixture-backed edge traffic into the `itinerary` CLI path. Done.
12. Add DOT/Socrata-backed edge-state fetcher for already matched DOT link IDs.
    Done.
13. Wire DOT-backed edge traffic into the `itinerary` CLI path. Done.
14. Add local DOT matching after the metadata artifact is stable. Done.
15. Add a `match-edges` CLI command that writes enriched edge metadata. Done.
16. Add EMA/cache persistence so DOT runs can reuse previous smoothed values.

## Current State

This package is not complete yet, but the edge artifact builder is in place:

- `Client.FetchEdge` fetches one directed OSRM `/route` edge.
- `Client.BuildEdgeMetadata` builds all directed non-diagonal edges for a stop
  list.
- `WriteEdgeMetadata` and `ReadEdgeMetadata` round-trip the artifact JSON.
- The CLI exposes this through `route-optimizer edge-metadata`.
- `BuildTrafficSnapshot` loops edges through an `EdgeStateFetcher` and applies
  EMA to current/previous multipliers.
- `ApplyEdgeTraffic` turns those smoothed states into an adjusted matrix
  through `route.ApplyTraffic`.
- `LoadFixtureEdgeStateFetcher` loads fake/demo traffic states from
  `testdata/edge_state_fixture.json`.
- `DOTClient` fetches NYC DOT Traffic Speeds rows from Socrata/SODA in batches
  by `link_id`.
- `DOTEdgeStateFetcher` converts matched live DOT speeds into per-edge traffic
  multipliers against OSRM baseline speed.
- `MatchDOTLinks` parses DOT `link_points` and enriches edges with local
  `matched_dot_link_ids`.
- `DOTClient.FetchAllTrafficRecords` can fetch the paginated DOT feed for local
  matching candidates.
- The `match-edges` command can write enriched edge metadata from a DOT fixture
  or live Socrata rows.
- The `itinerary` command can apply the fixture-backed traffic overlay with
  `-edge-metadata` and `-edge-state-fixture`.
- The `itinerary` command can apply DOT-backed traffic with `-edge-metadata`
  and `-dot-traffic` when edge metadata already has `matched_dot_link_ids`.
- Unit tests mock OSRM with `httptest`.
- Unit tests mock DOT/Socrata with `httptest`.
- Parser tests read a real recorded OSRM response from
  `testdata/osrm_route_response.json`.
- A real OSRM integration test exists but is skipped unless
  `PEPSI_RUN_OSRM_INTEGRATION=1`.

Regenerate the recorded fixture with:

```bash
PEPSI_RECORD_OSRM_FIXTURE=1 go test ./pepsi -run TestRecordOSRMRouteFixture -count=1 -v
```

The immediate next goal is making matching inspectable: tune thresholds on real
routes, add debug output for rejected links, and persist DOT/EMA state so demo
runs are repeatable.
