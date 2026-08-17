// Package httpapi owns the root HTTP transport and mounts versioned APIs.
package httpapi

import (
	"net/http"

	"route-optimizer-go/frontend"
	apiv1 "route-optimizer-go/internal/httpapi/v1"
)

type Server struct {
	router *http.ServeMux
}

// Limits are the configured planner bounds published by the HTTP API.
type Limits = apiv1.Limits

func (s *Server) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	s.router.ServeHTTP(w, r)
}

func NewServer(optimizer apiv1.RouteOptimizer, geocoder apiv1.AddressGeocoder, limits Limits) (*Server, error) {
	server := &Server{
		router: http.NewServeMux(),
	}
	server.router.Handle("/", frontend.Handler())

	if err := server.registerRoutes(optimizer, geocoder, limits); err != nil {
		return nil, err
	}
	return server, nil
}
