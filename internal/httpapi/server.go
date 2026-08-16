package httpapi

import (
	"net/http"
)

type Server struct {
	router *http.ServeMux
}

func (s *Server) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	s.router.ServeHTTP(w, r)
}

func NewServer() (*Server, error) {
	server := &Server {
		router : http.NewServeMux(),
	}
	server.router.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World\n"))
	}))

	server.registerRoutes()
	return server, nil
}


