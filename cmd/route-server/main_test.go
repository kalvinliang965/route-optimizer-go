package main

import (
	"context"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer("127.0.0.1:9090", handler)

	if server.Addr != "127.0.0.1:9090" {
		t.Fatalf("address = %q, want 127.0.0.1:9090", server.Addr)
	}
	if server.Handler == nil {
		t.Fatal("handler is nil")
	}
	if server.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("read-header timeout = %s, want %s", server.ReadHeaderTimeout, serverReadHeaderTimeout)
	}
	if server.ReadTimeout != serverReadTimeout {
		t.Fatalf("read timeout = %s, want %s", server.ReadTimeout, serverReadTimeout)
	}
	if server.WriteTimeout != serverWriteTimeout {
		t.Fatalf("write timeout = %s, want %s", server.WriteTimeout, serverWriteTimeout)
	}
	if server.IdleTimeout != serverIdleTimeout {
		t.Fatalf("idle timeout = %s, want %s", server.IdleTimeout, serverIdleTimeout)
	}
}

func TestRunHTTPServerStopsCleanlyOnSignal(t *testing.T) {
	server := newLifecycleTestServer()
	shutdownSignals := make(chan os.Signal, 1)
	shutdownSignals <- syscall.SIGTERM

	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(server, shutdownSignals)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHTTPServer() error = %v", err)
		}
		if server.shutdownCalls != 1 {
			t.Fatalf("shutdown calls = %d, want 1", server.shutdownCalls)
		}
		if server.closeCalls != 0 {
			t.Fatalf("close calls = %d, want 0", server.closeCalls)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runHTTPServer did not stop after signal")
	}
}

type lifecycleTestServer struct {
	stopped       chan struct{}
	shutdownCalls int
	closeCalls    int
}

func newLifecycleTestServer() *lifecycleTestServer {
	return &lifecycleTestServer{stopped: make(chan struct{})}
}

func (s *lifecycleTestServer) ListenAndServe() error {
	<-s.stopped
	return http.ErrServerClosed
}

func (s *lifecycleTestServer) Shutdown(context.Context) error {
	s.shutdownCalls++
	close(s.stopped)
	return nil
}

func (s *lifecycleTestServer) Close() error {
	s.closeCalls++
	return nil
}

func TestDefaultListenAddressUsesPortEnvironment(t *testing.T) {
	original, existed := os.LookupEnv("PORT")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("PORT", original)
			return
		}
		_ = os.Unsetenv("PORT")
	})

	if err := os.Unsetenv("PORT"); err != nil {
		t.Fatal(err)
	}
	if got := defaultListenAddress(); got != ":8080" {
		t.Fatalf("default address = %q, want :8080", got)
	}

	if err := os.Setenv("PORT", "3000"); err != nil {
		t.Fatal(err)
	}
	if got := defaultListenAddress(); got != ":3000" {
		t.Fatalf("Replit address = %q, want :3000", got)
	}

	if err := os.Setenv("PORT", "0.0.0.0:4000"); err != nil {
		t.Fatal(err)
	}
	if got := defaultListenAddress(); got != "0.0.0.0:4000" {
		t.Fatalf("explicit environment address = %q", got)
	}
}
