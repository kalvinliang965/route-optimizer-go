# Dynamic Traffic-Aware Routing Architecture

A high-performance routing and live traffic integration pipeline bridging Open Source Routing Machine (OSRM) with real-time New York City Department of Transportation (NYC DOT) telemetry via the Socrata API.

---

## 1. System Architecture

The core philosophy of this architecture is separating **static geometric routing** from **dynamic traffic overlay ingestion**. Because external public traffic APIs cannot be queried infinitely per route request, the system relies on an upfront mapping phase and a background polling loop.

```
+-----------------------------------------------------------------+
|                        1. Upfront Mapping                       |
|                                                                 |
|  [OSRM /route API] ---> Extracts Step Geometries & Street Names |
|                                |                                |
|                                v                                |
|  [Spatial Matcher] ---> Cross-references against NYC DOT        |
|                         Sensor Coordinates (link_points)        |
|                                |                                |
|                                v                                |
|  [Routing Matrix]  ---> Binds Matrix Edges to Target link_ids   |
+-----------------------------------------------------------------+
                                 |
                                 v
+-----------------------------------------------------------------+
|                     2. Runtime Dynamic Flow                     |
|                                                                 |
|  [Socrata API] <--- Batched HTTP Polling (link_id IN (...))     |
|       |                                                         |
|       v                                                         |
|  [In-Memory Cache] -> Updates Matrix Cell Weights Dynamically   |
|       |                                                         |
|       v                                                         |
|  [Routing Solver] -> Computes live traffic-adjusted paths       |
+-----------------------------------------------------------------+
```

### Component Breakdown
1. **Routing Backbone (OSRM):** Generates baseline path geometries, turn-by-turn steps, and raw free-flow durations/distances between coordinate nodes.
2. **Spatial Translation Layer:** Translates OSRM route steps and coordinate boundaries into corresponding NYC DOT `link_id` identifiers.
3. **Live Ingestion Layer (Socrata API):** Periodically polls the NYC DOT Traffic Speeds dataset in efficient batches using SoQL `IN` clauses to fetch live speeds and travel times.
4. **Dynamic Routing Matrix:** Overwrites static OSRM matrix edge costs with real-time traffic penalties.

---

## 2. Current Implementation State & Required Fixes

### What Works
* Successfully queries OSRM endpoints (`/route/v1/driving/` and `/table/v1/driving/`) to retrieve route legs, step details, and base duration matrices.
* Successfully queries the Socrata endpoint for specific `link_id` records using SoQL filter parameters (`$where`, `$order`, `$limit`).

### Identified Gaps & Required Refactoring
The current implementation of the matrix builder only captures aggregate durations (e.g., base travel time). To support real-time traffic adjustments, **the matrix generation step must be extended to capture and retain all underlying spatial link points / path geometry.**

#### Required Changes:
1. **Geometry Retention:** During OSRM route parsing, store the complete sequence of coordinate points and step names associated with each matrix edge rather than discarding them after calculating base duration.
2. **Upfront Spatial Indexing:** Implement a spatial matching function in Go to compare OSRM path segments against the bounding boxes or coordinate points of NYC DOT sensors (`link_points`).
3. **`link_id` Binding:** Map each routing matrix edge directly to a slice of target `link_id` strings (`[]string`).
4. **Batched Ingestion Integration:** Feed the compiled list of mapped `link_id`s into a central background polling worker that fetches updates from Socrata in a single aggregated batch request.
