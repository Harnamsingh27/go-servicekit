package grpcx

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server wraps grpc.Server with a built-in gRPC Health service and standard
// interceptors (request-ID propagation). Additional server options — e.g.
// auth interceptors — can be passed at construction time.
type Server struct {
	inner  *grpc.Server
	health *health.Server
	addr   string
}

// NewServer creates a gRPC server listening on addr. Extra grpc.ServerOptions
// (interceptors, credentials, etc.) are appended after the built-in ones.
func NewServer(addr string, opts ...grpc.ServerOption) *Server {
	base := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(requestIDUnaryInterceptor),
		grpc.ChainStreamInterceptor(requestIDStreamInterceptor),
	}
	srv := grpc.NewServer(append(base, opts...)...)
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hs)
	reflection.Register(srv)
	return &Server{inner: srv, health: hs, addr: addr}
}

// RegisterService delegates to the underlying grpc.Server.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.inner.RegisterService(desc, impl)
}

// SetServingStatus updates the health-check status for service. Pass "" to set
// the overall server status.
func (s *Server) SetServingStatus(service string, st grpc_health_v1.HealthCheckResponse_ServingStatus) {
	s.health.SetServingStatus(service, st)
}

// GRPCServer returns the underlying *grpc.Server for direct registration.
func (s *Server) GRPCServer() *grpc.Server { return s.inner }

// Serve starts the gRPC server and blocks until ctx is cancelled, then drains
// connections gracefully.
func (s *Server) Serve(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpcx: listen %s: %w", s.addr, err)
	}
	errCh := make(chan error, 1)
	go func() {
		if err := s.inner.Serve(lis); err != nil {
			errCh <- fmt.Errorf("grpcx: serve: %w", err)
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.inner.GracefulStop()
	}
	return <-errCh
}
