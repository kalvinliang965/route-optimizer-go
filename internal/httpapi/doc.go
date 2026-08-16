// Package httpapi is the transport boundary for the future HTTP server.
//
// It is intentionally empty apart from this package declaration so the server
// can be implemented as a learning exercise. Handlers should decode JSON,
// validate transport-level concerns, call planner.Service.Optimize with the
// request context, and encode planner results. Route calculation must remain in
// optimizer and workflow coordination must remain in planner.
package httpapi
