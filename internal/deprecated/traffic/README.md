# Deprecated Traffic Experiment

Status: **preserved but stalled**. This package is not called by `route-cli`,
`planner.Service`, or the future HTTP server.

It keeps only the unique traffic research code. The former duplicate route
solver, geocoder, OSRM matrix client, cache, maps output, configuration, and CLI
were removed. This package reuses `optimizer.Stop` and `storage` where it needs
active domain types or JSON persistence.

## What Is Preserved

- directed OSRM `/route` geometry and step metadata for every stop pair;
- local matching between route geometry and NYC DOT `link_points`;
- bounded recent DOT/Socrata record fetching;
- conversion of matched DOT speeds into edge multipliers;
- fixture-backed edge states;
- EMA smoothing of current and previous multipliers;
- application of multipliers to a baseline duration matrix; and
- unit, fixture, and mocked HTTP tests for those components.

## Experimental Flow

```text
optimizer.Stop list
  -> OSRM directed edge geometry
  -> local NYC DOT link matching
  -> current speed / baseline speed
  -> traffic multiplier and optional EMA
  -> adjusted duration matrix
  -> planner/optimizer (not currently connected)
```

The multiplier is applied element by element:

```text
adjusted_duration[i][j] = baseline_duration[i][j] * multiplier[i][j]
```

## Why It Is Stalled

- The NYC DOT dataset is a very large append-only historical feed. Treating it
  as a small current snapshot causes slow global queries.
- DOT sensors cover selected major roads, so many valid routes have no match.
- OSRM supplies the estimated geometry while Google Maps executes the final
  itinerary and may choose different roads.
- Edge artifacts do not yet include an ordered-stop/provider fingerprint, so
  old matches cannot safely be reused for a different request.
- EMA state is not persisted across independent process runs.

## Safe Resume Path

1. Define a small planner interface such as `MatrixAdjuster`.
2. Add stop/provider fingerprints and a schema version to edge artifacts.
3. Fetch a bounded newest-first DOT window and reject stale source timestamps.
4. Persist previous multipliers if EMA is still useful.
5. Add the experiment as an optional adapter during dependency wiring.
6. Keep all traffic and provider decisions out of HTTP handlers.

The historical fixture recorder is optional and network-dependent:

```bash
PEPSI_RECORD_OSRM_FIXTURE=1 \
  go test ./internal/deprecated/traffic -run TestRecordOSRMRouteFixture -count=1 -v
```
