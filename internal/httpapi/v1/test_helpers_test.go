package v1

import (
	"context"

	"route-optimizer-go/internal/optimizer"
)

func testLimits(maxStops int) Limits {
	return Limits{DefaultTopK: 5, MaxTopK: 20, MaxStops: maxStops}
}

type fakeAddressGeocoder struct {
	stops  map[string]optimizer.Stop
	errors map[string]error
	calls  []string
}

func (f *fakeAddressGeocoder) Geocode(_ context.Context, address string) (optimizer.Stop, error) {
	f.calls = append(f.calls, address)
	if err := f.errors[address]; err != nil {
		return optimizer.Stop{}, err
	}
	if stop, found := f.stops[address]; found {
		return stop, nil
	}
	return optimizer.Stop{Name: address, Lat: 40, Lon: -73}, nil
}
