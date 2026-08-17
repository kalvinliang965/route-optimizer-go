package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"route-optimizer-go/internal/optimizer"
)

type geocodeRequest struct {
	Addresses []string `json:"addresses"`
}

type geocodeResponse struct {
	Results []geocodeResult `json:"results"`
}

type geocodeResult struct {
	Index   int             `json:"index"`
	Address string          `json:"address"`
	Stop    *optimizer.Stop `json:"stop,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func (s *Server) geocode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request geocodeRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	if len(request.Addresses) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "at least one address is required")
		return
	}
	if len(request.Addresses) > s.limits.MaxStops {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("too many addresses: %d (max %d)", len(request.Addresses), s.limits.MaxStops))
		return
	}

	response := geocodeResponse{Results: make([]geocodeResult, len(request.Addresses))}
	for index, address := range request.Addresses {
		address = strings.TrimSpace(address)
		result := geocodeResult{Index: index, Address: address}
		if address == "" {
			result.Error = "address is empty"
			response.Results[index] = result
			continue
		}

		stop, err := s.geocoder.Geocode(r.Context(), address)
		if err != nil {
			result.Error = err.Error()
			response.Results[index] = result
			continue
		}
		stop.ID = fmt.Sprintf("stop-%d", index)
		result.Stop = &stop
		response.Results[index] = result
	}

	_ = writeJSON(w, http.StatusOK, response)
}
