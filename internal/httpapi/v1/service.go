package v1

import (
	"context"

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
)

// RouteOptimizer is the application behavior required by the optimize
// endpoint. The HTTP layer owns this narrow interface; planner.Service is its
// production implementation.
type RouteOptimizer interface {
	Optimize(context.Context, planner.OptimizeRequest) (planner.OptimizeResult, error)
}

// AddressGeocoder is the provider behavior required by the geocode endpoint.
// The HTTP layer adds row indexes and request-specific stop IDs.
type AddressGeocoder interface {
	Geocode(context.Context, string) (optimizer.Stop, error)
}
