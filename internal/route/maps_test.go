package route

import (
	"net/url"
	"strings"
	"testing"
)

func TestGoogleMapsDirectionsURL(t *testing.T) {
	stops := []Stop{
		{Name: "A", Lat: 40.712800, Lon: -74.006000},
		{Name: "B", Lat: 40.758000, Lon: -73.985500},
		{Name: "C", Lat: 40.748400, Lon: -73.985700},
	}

	tests := []struct {
		name    string
		path    []int
		wantErr bool
		check   func(t *testing.T, raw string)
	}{
		{
			name: "two stops origin and destination only",
			path: []int{0, 1},
			check: func(t *testing.T, raw string) {
				q := parseQuery(t, raw)
				if q.Get("origin") != "40.712800,-74.006000" {
					t.Errorf("origin = %q", q.Get("origin"))
				}
				if q.Get("destination") != "40.758000,-73.985500" {
					t.Errorf("destination = %q", q.Get("destination"))
				}
				if q.Get("waypoints") != "" {
					t.Errorf("waypoints = %q; want empty", q.Get("waypoints"))
				}
				if q.Get("travelmode") != "driving" {
					t.Errorf("travelmode = %q", q.Get("travelmode"))
				}
			},
		},
		{
			name: "three stops with one waypoint",
			path: []int{0, 2, 1},
			check: func(t *testing.T, raw string) {
				q := parseQuery(t, raw)
				if q.Get("waypoints") != "40.748400,-73.985700" {
					t.Errorf("waypoints = %q", q.Get("waypoints"))
				}
			},
		},
		{
			name: "four stops with two waypoints",
			path: []int{0, 1, 2, 0},
			check: func(t *testing.T, raw string) {
				q := parseQuery(t, raw)
				want := "40.758000,-73.985500|40.748400,-73.985700"
				if q.Get("waypoints") != want {
					t.Errorf("waypoints = %q; want %q", q.Get("waypoints"), want)
				}
			},
		},
		{
			name:    "empty path",
			path:    nil,
			wantErr: true,
		},
		{
			name:    "invalid index",
			path:    []int{0, 99},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GoogleMapsDirectionsURL(stops, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("GoogleMapsDirectionsURL: %v", err)
			}
			if !strings.HasPrefix(got, "https://www.google.com/maps/dir/?") {
				t.Fatalf("unexpected URL prefix: %s", got)
			}
			tt.check(t, got)
		})
	}
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return u.Query()
}
