package v1

import (
	"fmt"
	"net/http"
)

// Limits are the planner limits exposed to HTTP clients. The server uses the
// same values to reject work that cannot be accepted by the configured planner.
type Limits struct {
	DefaultTopK int `json:"default_top_k"`
	MaxTopK     int `json:"max_top_k"`
	MaxStops    int `json:"max_stops"`
}

func (l Limits) validate() error {
	if l.DefaultTopK < 1 {
		return fmt.Errorf("default top_k must be >= 1")
	}
	if l.MaxTopK < l.DefaultTopK {
		return fmt.Errorf("max top_k must be >= default top_k")
	}
	if l.MaxStops < 2 {
		return fmt.Errorf("max stops must be >= 2")
	}
	return nil
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_ = writeJSON(w, http.StatusOK, s.limits)
}
