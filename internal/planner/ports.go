// Package planner coordinates the route optimization use case.
package planner

import (
	"context"

	"route-optimizer-go/internal/optimizer"
)

// Geocoder resolves a user-entered address into a stop.
type Geocoder interface {
	Geocode(ctx context.Context, address string) (optimizer.Stop, error)
}

// MatrixProvider supplies directed travel durations for an ordered stop list.
type MatrixProvider interface {
	Durations(ctx context.Context, stops []optimizer.Stop) (optimizer.Matrix, error)
}

// DirectionsBuilder creates an execution link for an already-selected order.
type DirectionsBuilder interface {
	DirectionsURL(stops []optimizer.Stop, path []int) (string, error)
}
