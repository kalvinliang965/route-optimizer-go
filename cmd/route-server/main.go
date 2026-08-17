package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"route-optimizer-go/internal/app"
	"route-optimizer-go/internal/config"
	"route-optimizer-go/internal/httpapi"
	"route-optimizer-go/internal/maps"
	"route-optimizer-go/internal/optimizer"
	"route-optimizer-go/internal/planner"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 15 * time.Second
	serverWriteTimeout      = 90 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 15 * time.Second
)

type httpServerLifecycle interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

func main() {
	addr := flag.String("addr", defaultListenAddress(), "HTTP listen address")
	configPath := flag.String("config", "", "path to YAML config; empty uses built-in defaults")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		loaded, err := config.Load(*configPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		cfg = loaded
	}
	geocoder := app.NewGeocoder(cfg, cfg.Providers.NominatimBaseURL, log.Printf)
	service := planner.Service{
		Solver:         optimizer.NewSolver(cfg.Planner.MaxStops, cfg.Planner.MaxTopK),
		Geocoder:       geocoder,
		MatrixProvider: app.NewMatrixProvider(cfg, cfg.Providers.OSRMBaseURL, log.Printf),
		Directions:     maps.Google{},
		DefaultTopK:    cfg.Planner.DefaultTopK,
	}

	server, err := httpapi.NewServer(service, geocoder, httpapi.Limits{
		DefaultTopK: cfg.Planner.DefaultTopK,
		MaxTopK:     cfg.Planner.MaxTopK,
		MaxStops:    cfg.Planner.MaxStops,
	})
	if err != nil {
		log.Fatalf("failed to construct server: %v", err)
	}
	httpServer := newHTTPServer(*addr, server)
	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	log.Printf("server is listening on %s", *addr)
	if err := runHTTPServer(httpServer, shutdownSignals); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}

func runHTTPServer(server httpServerLifecycle, shutdownSignals <-chan os.Signal) error {
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case receivedSignal := <-shutdownSignals:
		log.Printf("received %s; shutting down", receivedSignal)

		ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		shutdownErr := server.Shutdown(ctx)
		cancel()
		if shutdownErr != nil {
			// Shutdown can time out while a handler is still active. Close makes
			// sure the listener and remaining connections do not leak.
			_ = server.Close()
		}

		serveErr := <-serveErrors
		if shutdownErr != nil {
			return fmt.Errorf("graceful shutdown: %w", shutdownErr)
		}
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}

func defaultListenAddress() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return ":8080"
	}
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}
