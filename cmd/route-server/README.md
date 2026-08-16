# Route server exercise

This directory is reserved for the HTTP server you will build. Start with a
`main.go` that constructs the adapters and `planner.Service`, then passes that
service to handlers in `internal/httpapi`.

Suggested first endpoints:

```text
GET  /healthz
POST /v1/optimize
```

Keep `main.go` limited to configuration, dependency construction, server
timeouts, signal handling, and graceful shutdown. JSON decoding and HTTP status
codes belong in `internal/httpapi`; route calculation belongs in
`internal/optimizer`.
