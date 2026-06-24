package httpx

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultReadTimeout     = 10 * time.Second
	defaultWriteTimeout    = 30 * time.Second
	defaultIdleTimeout     = 120 * time.Second
	defaultShutdownTimeout = 10 * time.Second
)

// Server wraps net/http.Server with production-safe timeouts and graceful
// shutdown driven by context cancellation.
type Server struct {
	inner           *http.Server
	shutdownTimeout time.Duration
}

// NewServer creates an HTTP server bound to addr with sane defaults.
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		inner: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  defaultReadTimeout,
			WriteTimeout: defaultWriteTimeout,
			IdleTimeout:  defaultIdleTimeout,
		},
		shutdownTimeout: defaultShutdownTimeout,
	}
}

// WithShutdownTimeout overrides the default 10 s graceful-shutdown window.
func (s *Server) WithShutdownTimeout(d time.Duration) *Server {
	s.shutdownTimeout = d
	return s
}

// ListenAndServe starts the server and blocks until ctx is cancelled. It then
// initiates a graceful shutdown, waiting up to the configured shutdown timeout
// for in-flight requests to complete.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.inner.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("httpx: serve: %w", err)
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.inner.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("httpx: graceful shutdown: %w", err)
	}
	return <-errCh
}
