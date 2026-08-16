// Package maps builds execution links for already-optimized stop orders.
package maps

import (
	"fmt"
	"net/url"
	"strings"

	"route-optimizer-go/internal/optimizer"
)

// Google builds Google Maps direction links. It does not optimize stop order.
type Google struct{}

func (Google) DirectionsURL(stops []optimizer.Stop, path []int) (string, error) {
	if len(path) < 2 {
		return "", fmt.Errorf("route path must contain an origin and destination")
	}
	for _, index := range path {
		if index < 0 || index >= len(stops) {
			return "", fmt.Errorf("invalid stop index %d", index)
		}
	}

	params := url.Values{}
	params.Set("api", "1")
	params.Set("origin", formatCoordinate(stops[path[0]]))
	params.Set("destination", formatCoordinate(stops[path[len(path)-1]]))
	params.Set("travelmode", "driving")

	if len(path) > 2 {
		waypoints := make([]string, 0, len(path)-2)
		for _, index := range path[1 : len(path)-1] {
			waypoints = append(waypoints, formatCoordinate(stops[index]))
		}
		params.Set("waypoints", strings.Join(waypoints, "|"))
	}

	return "https://www.google.com/maps/dir/?" + params.Encode(), nil
}

func formatCoordinate(stop optimizer.Stop) string {
	return fmt.Sprintf("%.6f,%.6f", stop.Lat, stop.Lon)
}
