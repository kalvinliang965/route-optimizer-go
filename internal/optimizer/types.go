// Package optimizer contains the pure route-order calculation domain.
// It deliberately has no HTTP, filesystem, configuration, or presentation dependencies.
package optimizer

// Stop is one location in an optimization request. Index 0 is the depot.
type Stop struct {
	ID   string  `json:"id,omitempty"`
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

// Matrix stores directed travel costs in seconds: matrix[from][to].
type Matrix [][]float64

// Route is one ranked round trip. Path contains stop indexes and repeats the
// depot at the end, for example [0, 2, 1, 0].
type Route struct {
	Path            []int   `json:"path"`
	DurationSeconds float64 `json:"duration_seconds"`
}
