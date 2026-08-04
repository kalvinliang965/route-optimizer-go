package route

import (
	"fmt"
	"net/url"
	"strings"
)

// GoogleMapsDirectionsURL builds a free Google Maps deep link for a stop order.
// path entries are indices into stops: first = origin, last = destination, middle = waypoints.
func GoogleMapsDirectionsURL(stops []Stop, path []int) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("empty route path")
	}
	for _, idx := range path {
		if idx < 0 || idx >= len(stops) {
			return "", fmt.Errorf("invalid stop index %d", idx)
		}
	}

	params := url.Values{}
	params.Set("api", "1")
	params.Set("origin", formatCoord(stops[path[0]]))
	params.Set("destination", formatCoord(stops[path[len(path)-1]]))
	params.Set("travelmode", "driving")

	if len(path) > 2 {
		waypoints := make([]string, 0, len(path)-2)
		for _, idx := range path[1 : len(path)-1] {
			waypoints = append(waypoints, formatCoord(stops[idx]))
		}
		params.Set("waypoints", strings.Join(waypoints, "|"))
	}

	return "https://www.google.com/maps/dir/?" + params.Encode(), nil
}

func formatCoord(s Stop) string {
	return fmt.Sprintf("%.6f,%.6f", s.Lat, s.Lon)
}
