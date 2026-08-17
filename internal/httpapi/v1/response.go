package v1

import (
	"encoding/json"
	"net/http"
)

const maxRequestBytes = 1 << 20

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}

func writeError(w http.ResponseWriter, status int, message string) {
	_ = writeJSON(w, status, errorResponse{Error: message})
}
