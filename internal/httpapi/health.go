package httpapi

import "net/http"

// check if server is still running
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{status: "ok"}`))
}