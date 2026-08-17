package httpapi

import (
	"fmt"
	"net/http"

	apiv1 "route-optimizer-go/internal/httpapi/v1"
)

func (s *Server) registerRoutes(optimizer apiv1.RouteOptimizer, geocoder apiv1.AddressGeocoder, limits Limits) error {
	s.router.HandleFunc("/healthz", s.health)

	v1Server, err := apiv1.NewServer(optimizer, geocoder, limits)
	if err != nil {
		return fmt.Errorf("construct v1 API: %w", err)
	}
	s.router.Handle("/v1/", http.StripPrefix("/v1", v1Server))
	return nil
}
