package planner

import (
	"context"
	"reflect"
	"testing"

	"route-optimizer-go/internal/optimizer"
)

func TestServiceUsesProvidedMatrixAndVariableTopK(t *testing.T) {
	service := Service{
		Solver:      optimizer.NewSolver(10, 10),
		Directions:  fakeDirections{},
		DefaultTopK: 5,
	}
	request := OptimizeRequest{
		Stops: []optimizer.Stop{
			{ID: "depot", Name: "Depot"},
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
		},
		DurationMatrixSeconds: optimizer.Matrix{{0, 1, 5}, {4, 0, 1}, {1, 5, 0}},
		TopK:                  2,
	}

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if result.TopK != 2 || result.MatrixSource != "provided" || len(result.Routes) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(result.Routes[0].Path, []int{0, 1, 2, 0}) {
		t.Fatalf("first path = %#v", result.Routes[0].Path)
	}
	if result.Routes[0].DirectionsURL != "maps://0-1-2-0" {
		t.Fatalf("directions URL = %q", result.Routes[0].DirectionsURL)
	}
}

func TestServiceResolvesAddressesAndBuildsMatrix(t *testing.T) {
	service := Service{
		Solver:         optimizer.NewSolver(10, 10),
		Geocoder:       fakeGeocoder{},
		MatrixProvider: fakeMatrixProvider{},
		DefaultTopK:    1,
	}
	result, err := service.Optimize(context.Background(), OptimizeRequest{Addresses: []string{"Depot", "A"}})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if result.MatrixSource != "provider" || len(result.Stops) != 2 || result.Stops[0].ID != "stop-0" {
		t.Fatalf("result = %#v", result)
	}
}

type fakeGeocoder struct{}

func (fakeGeocoder) Geocode(_ context.Context, address string) (optimizer.Stop, error) {
	return optimizer.Stop{Name: address, Lat: 40, Lon: -73}, nil
}

type fakeMatrixProvider struct{}

func (fakeMatrixProvider) Durations(_ context.Context, stops []optimizer.Stop) (optimizer.Matrix, error) {
	return optimizer.Matrix{{0, 10}, {12, 0}}, nil
}

type fakeDirections struct{}

func (fakeDirections) DirectionsURL(_ []optimizer.Stop, path []int) (string, error) {
	result := "maps://"
	for index, value := range path {
		if index > 0 {
			result += "-"
		}
		result += string(rune('0' + value))
	}
	return result, nil
}
