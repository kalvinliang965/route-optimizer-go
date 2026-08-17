package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
)

type optimizeRequest struct {
	Stops []optimizeStop `json:"stops"`
	TopK  int            `json:"top_k"`
}

// optimizeStop uses pointers for coordinates so omitted/null fields are not
// mistaken for the valid geographic coordinate zero.
type optimizeStop struct {
	ID   string   `json:"id,omitempty"`
	Name string   `json:"name"`
	Lon  *float64 `json:"lon"`
	Lat  *float64 `json:"lat"`
}

func (s *Server) optimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request optimizeRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	stops, err := s.validateOptimizeRequest(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	result, err := s.optimizer.Optimize(r.Context(), planner.OptimizeRequest{
		Stops: stops,
		TopK:  request.TopK,
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if err := writeJSON(w, http.StatusOK, result); err != nil {
		// Encoding planner.OptimizeResult should not fail. At this point the
		// response may already be committed, so there is no second safe body.
		return
	}
}

func (s *Server) validateOptimizeRequest(request optimizeRequest) ([]optimizer.Stop, error) {
	if len(request.Stops) < 2 {
		return nil, fmt.Errorf("at least two stops are required (a depot and one destination)")
	}
	if len(request.Stops) > s.limits.MaxStops {
		return nil, fmt.Errorf("too many stops: %d (max %d)", len(request.Stops), s.limits.MaxStops)
	}
	if request.TopK < 0 {
		return nil, fmt.Errorf("top_k must be >= 1 when provided, got %d", request.TopK)
	}
	if request.TopK > s.limits.MaxTopK {
		return nil, fmt.Errorf("top_k must be <= %d, got %d", s.limits.MaxTopK, request.TopK)
	}

	stops := make([]optimizer.Stop, len(request.Stops))
	ids := make(map[string]int, len(request.Stops))
	for index, input := range request.Stops {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, fmt.Errorf("stop %d name is required", index)
		}
		if input.Lat == nil {
			return nil, fmt.Errorf("stop %d latitude is required", index)
		}
		if math.IsNaN(*input.Lat) || math.IsInf(*input.Lat, 0) || *input.Lat < -90 || *input.Lat > 90 {
			return nil, fmt.Errorf("stop %d latitude must be finite and between -90 and 90", index)
		}
		if input.Lon == nil {
			return nil, fmt.Errorf("stop %d longitude is required", index)
		}
		if math.IsNaN(*input.Lon) || math.IsInf(*input.Lon, 0) || *input.Lon < -180 || *input.Lon > 180 {
			return nil, fmt.Errorf("stop %d longitude must be finite and between -180 and 180", index)
		}

		id := strings.TrimSpace(input.ID)
		if input.ID != "" && id == "" {
			return nil, fmt.Errorf("stop %d id cannot be blank", index)
		}
		if previous, found := ids[id]; id != "" && found {
			return nil, fmt.Errorf("stop %d id %q duplicates stop %d", index, id, previous)
		}
		if id != "" {
			ids[id] = index
		}

		stops[index] = optimizer.Stop{ID: id, Name: name, Lat: *input.Lat, Lon: *input.Lon}
	}
	return stops, nil
}
