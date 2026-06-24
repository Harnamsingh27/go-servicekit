package grpcx

import (
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// ServerOption configures a grpcx Server.
type ServerOption func(*serverConfig)

type serverConfig struct {
	unary  []grpc.UnaryServerInterceptor
	stream []grpc.StreamServerInterceptor
	opts   []grpc.ServerOption
}

// WithUnaryInterceptors appends unary server interceptors.
// They are chained in the order provided (first = outermost wrapper).
func WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) ServerOption {
	return func(c *serverConfig) { c.unary = append(c.unary, interceptors...) }
}

// WithStreamInterceptors appends stream server interceptors.
func WithStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) ServerOption {
	return func(c *serverConfig) { c.stream = append(c.stream, interceptors...) }
}

// WithGRPCOptions appends raw grpc.ServerOption values.
func WithGRPCOptions(opts ...grpc.ServerOption) ServerOption {
	return func(c *serverConfig) { c.opts = append(c.opts, opts...) }
}

// Server wraps grpc.Server with health checking and a structured interceptor chain.
type Server struct {
	inner  *grpc.Server
	health *health.Server
}

// NewServer creates a Server. It automatically registers the gRPC health service.
func NewServer(opts ...ServerOption) *Server {
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}

	serverOpts := append([]grpc.ServerOption{}, cfg.opts...)
	if len(cfg.unary) > 0 {
		serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(cfg.unary...))
	}
	if len(cfg.stream) > 0 {
		serverOpts = append(serverOpts, grpc.ChainStreamInterceptor(cfg.stream...))
	}

	gs := grpc.NewServer(serverOpts...)
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, hs)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	return &Server{inner: gs, health: hs}
}

// RegisterService registers a gRPC service implementation and marks it as SERVING
// in the health server.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.inner.RegisterService(desc, impl)
	s.health.SetServingStatus(desc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
}

// Serve accepts connections on l and blocks until the server stops.
func (s *Server) Serve(l net.Listener) error {
	return s.inner.Serve(l)
}

// GracefulStop marks all services NOT_SERVING, then waits for active RPCs to
// finish before stopping.
func (s *Server) GracefulStop() {
	s.health.Shutdown()
	s.inner.GracefulStop()
}

// Stop forcefully terminates all connections and stops the server.
func (s *Server) Stop() {
	s.health.Shutdown()
	s.inner.Stop()
}
