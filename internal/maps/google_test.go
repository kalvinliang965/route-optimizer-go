package maps

import (
	"net/url"
	"strings"
	"testing"

	"route-optimizer-go/internal/optimizer"
)

func TestGoogleDirectionsURLUsesCalculatedOrderAndReturnDepot(t *testing.T) {
	stops := []optimizer.Stop{
		{Name: "Depot", Lat: 40.1, Lon: -73.1},
		{Name: "A", Lat: 40.2, Lon: -73.2},
		{Name: "B", Lat: 40.3, Lon: -73.3},
	}
	raw, err := (Google{}).DirectionsURL(stops, []int{0, 2, 1, 0})
	if err != nil {
		t.Fatalf("DirectionsURL: %v", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	query := parsed.Query()
	if query.Get("origin") != "40.100000,-73.100000" || query.Get("destination") != "40.100000,-73.100000" {
		t.Fatalf("origin/destination = %q/%q", query.Get("origin"), query.Get("destination"))
	}
	if query.Get("waypoints") != "40.300000,-73.300000|40.200000,-73.200000" {
		t.Fatalf("waypoints = %q", query.Get("waypoints"))
	}
	if !strings.HasPrefix(raw, "https://www.google.com/maps/dir/?") {
		t.Fatalf("URL = %q", raw)
	}
}
