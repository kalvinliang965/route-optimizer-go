package httpapi

func (s *Server) registerRoutes() {
	s.router.HandleFunc("/healthz", s.health)
}