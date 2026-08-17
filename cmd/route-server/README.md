# Route server

`main.go` is the process entry point. It constructs the root HTTP handler and
starts listening on port `8080`. Routing and responses remain in
`internal/httpapi`.

Current layout:

```text
cmd/route-server/main.go       process and listener
frontend/                      embedded HTML, CSS, and vanilla JavaScript
internal/httpapi/              root routes such as /healthz
internal/httpapi/v1/           version 1 API routes and handlers
```

Run it with:

```bash
go run ./cmd/route-server -addr :8080 -config config.example.yaml
curl -i http://localhost:8080/healthz
curl -i \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "addresses": [
      "Times Square, New York, NY",
      "New York City Hall, New York, NY"
    ]
  }' \
  http://localhost:8080/v1/geocode
curl -i \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "stops": [
      {"id":"depot","name":"Depot","lat":40.75,"lon":-73.98},
      {"id":"a","name":"Stop A","lat":40.72,"lon":-74.00}
    ],
    "top_k": 5
  }' \
  http://localhost:8080/v1/optimize
```

The page at `/` provides the complete address → geocode → optimize → Google
Maps workflow. It uses one batch resolve button, shows provider errors on their
exact rows, and keeps labels/notes separate from geocoder input. Every row can
accept either an address or coordinates directly. Successful address lookups
become editable coordinate fields; directly entered coordinates bypass
geocoding but still use the routing provider and matrix cache. There is no
frontend installation or build step.

`/healthz` returns `200 OK`. `GET /v1/config` publishes the configured default
and maximum `top_k` plus the stop limit used by the UI. `/v1/geocode` returns an
ordered success/error item for each address, allowing a frontend to mark a
specific bad row.
`/v1/optimize` calls `planner.Service` and returns ranked routes as JSON.
Successful geocodes and OSRM matrices are reused from separate expiring caches
under `data/cache`. Complete API request/result artifacts are not persisted, but
the derived cache files contain resolved locations and coordinates. They are
written with owner-only permissions and can be deleted safely. Omit `-config`
to use built-in defaults: 90 days for geocodes and 30 days for matrices.
Expired results remain fallbacks when a provider cannot refresh them.

The default public Nominatim endpoint is limited to one serialized cache-miss
request per second inside this process. Trigger `/v1/geocode` from an explicit
submit action, never autocomplete, display OpenStreetMap attribution, and do not
send confidential addresses. See the
[Nominatim usage policy](https://operations.osmfoundation.org/policies/nominatim/).
The default public OSRM endpoint uses HTTPS and likewise permits at most one
process-wide matrix cache-miss request per second. One OSRM table request builds
the complete matrix; fresh cache hits bypass the limiter. See the
[OSRM demo server policy](https://github.com/Project-OSRM/osrm-backend/wiki/Demo-server).

On Replit, omit `-addr` to use its `PORT` environment variable:

```bash
go run ./cmd/route-server -config config.example.yaml
```

Publish it as an Autoscale or Reserved VM app, not a Static app, because the Go
API must run. Replit's published filesystem is not persistent, so treat the
disk provider cache as best-effort rather than durable storage. See
[Replit deployment troubleshooting](https://docs.replit.com/build/troubleshooting).

Keep `main.go` limited to configuration, dependency construction, server
timeouts, signal handling, and graceful shutdown. JSON decoding and HTTP status
codes belong in `internal/httpapi`; route calculation belongs in
`internal/optimizer` and workflow coordination belongs in `internal/planner`.
