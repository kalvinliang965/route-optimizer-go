// Package v1 contains version 1 of the route optimizer HTTP API.
package v1

import (
	"fmt"
	"net/http"
)

type Server struct {
	router    *http.ServeMux
	optimizer RouteOptimizer
	geocoder  AddressGeocoder
	limits    Limits
}

func NewServer(optimizer RouteOptimizer, geocoder AddressGeocoder, limits Limits) (*Server, error) {
	if optimizer == nil {
		return nil, fmt.Errorf("route optimizer is required")
	}
	if geocoder == nil {
		return nil, fmt.Errorf("address geocoder is required")
	}
	if err := limits.validate(); err != nil {
		return nil, err
	}
	server := &Server{
		router:    http.NewServeMux(),
		optimizer: optimizer,
		geocoder:  geocoder,
		limits:    limits,
	}
	server.registerRoutes()
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.router.HandleFunc("/config", s.config)
	s.router.HandleFunc("/geocode", s.geocode)
	s.router.HandleFunc("/optimize", s.optimize)
}
