package planner

import (
	"context"
	"fmt"
	"strings"

	"route-optimizer-go/internal/optimizer"
)

// OptimizeRequest accepts either addresses or resolved stops. A caller may
// provide a matrix; otherwise the configured MatrixProvider is used.
type OptimizeRequest struct {
	Addresses             []string
	Stops                 []optimizer.Stop
	DurationMatrixSeconds optimizer.Matrix
	TopK                  int
}

type PlannedRoute struct {
	Rank            int              `json:"rank"`
	Path            []int            `json:"path"`
	OrderedStops    []optimizer.Stop `json:"ordered_stops"`
	DurationSeconds float64          `json:"duration_seconds"`
	DirectionsURL   string           `json:"directions_url,omitempty"`
}

type OptimizeResult struct {
	Stops        []optimizer.Stop `json:"stops"`
	MatrixSource string           `json:"matrix_source"`
	// Matrix is available to non-HTTP adapters such as the CLI for artifact
	// output, but is intentionally omitted when OptimizeResult is encoded.
	Matrix optimizer.Matrix `json:"-"`
	TopK   int              `json:"top_k"`
	Routes []PlannedRoute   `json:"routes"`
}

// Service owns the application workflow. Transport layers such as a CLI or
// HTTP handler should call this instead of coordinating providers themselves.
type Service struct {
	Solver         optimizer.Solver
	Geocoder       Geocoder
	MatrixProvider MatrixProvider
	Directions     DirectionsBuilder
	DefaultTopK    int
}

func (s Service) Optimize(ctx context.Context, request OptimizeRequest) (OptimizeResult, error) {
	stops, err := s.resolveStops(ctx, request)
	if err != nil {
		return OptimizeResult{}, err
	}

	matrix := request.DurationMatrixSeconds
	matrixSource := "provided"
	if len(matrix) == 0 {
		if s.MatrixProvider == nil {
			return OptimizeResult{}, fmt.Errorf("duration matrix is required when no matrix provider is configured")
		}
		matrix, err = s.MatrixProvider.Durations(ctx, stops)
		if err != nil {
			return OptimizeResult{}, fmt.Errorf("build duration matrix: %w", err)
		}
		matrixSource = "provider"
	}

	topK := request.TopK
	if topK == 0 {
		topK = s.DefaultTopK
		if topK == 0 {
			topK = optimizer.DefaultTopK
		}
	}

	routes, err := s.Solver.Solve(stops, matrix, topK)
	if err != nil {
		return OptimizeResult{}, fmt.Errorf("solve routes: %w", err)
	}

	planned := make([]PlannedRoute, 0, len(routes))
	for index, route := range routes {
		ordered := make([]optimizer.Stop, len(route.Path))
		for i, stopIndex := range route.Path {
			ordered[i] = stops[stopIndex]
		}

		mapsURL := ""
		if s.Directions != nil {
			mapsURL, err = s.Directions.DirectionsURL(stops, route.Path)
			if err != nil {
				return OptimizeResult{}, fmt.Errorf("build directions for route %d: %w", index+1, err)
			}
		}

		planned = append(planned, PlannedRoute{
			Rank:            index + 1,
			Path:            append([]int(nil), route.Path...),
			OrderedStops:    ordered,
			DurationSeconds: route.DurationSeconds,
			DirectionsURL:   mapsURL,
		})
	}

	return OptimizeResult{
		Stops:        append([]optimizer.Stop(nil), stops...),
		MatrixSource: matrixSource,
		Matrix:       copyMatrix(matrix),
		TopK:         topK,
		Routes:       planned,
	}, nil
}

func copyMatrix(matrix optimizer.Matrix) optimizer.Matrix {
	copy := make(optimizer.Matrix, len(matrix))
	for index := range matrix {
		copy[index] = append([]float64(nil), matrix[index]...)
	}
	return copy
}

func (s Service) resolveStops(ctx context.Context, request OptimizeRequest) ([]optimizer.Stop, error) {
	if len(request.Addresses) > 0 && len(request.Stops) > 0 {
		return nil, fmt.Errorf("provide either addresses or stops, not both")
	}
	if len(request.Stops) > 0 {
		return append([]optimizer.Stop(nil), request.Stops...), nil
	}
	if len(request.Addresses) == 0 {
		return nil, fmt.Errorf("at least one address or stop is required")
	}
	if s.Geocoder == nil {
		return nil, fmt.Errorf("addresses require a configured geocoder")
	}

	stops := make([]optimizer.Stop, len(request.Addresses))
	for index, address := range request.Addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			return nil, fmt.Errorf("address %d is empty", index)
		}
		stop, err := s.Geocoder.Geocode(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("geocode address %d %q: %w", index, address, err)
		}
		if stop.ID == "" {
			stop.ID = fmt.Sprintf("stop-%d", index)
		}
		stops[index] = stop
	}
	return stops, nil
}
