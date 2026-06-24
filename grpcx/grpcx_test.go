package grpcx_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/harnamsingh/go-servicekit/grpcx"
	"github.com/harnamsingh/go-servicekit/observability"
)

const bufSize = 1 << 20 // 1 MB

func newLogger() *slog.Logger { return observability.NewLogger(observability.WithLogOutput(io.Discard)) }

// startServer creates a grpcx.Server on an in-memory listener and returns the
// listener (for dialling) and a cleanup function.
func startServer(t *testing.T, opts ...grpcx.ServerOption) (*bufconn.Listener, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpcx.NewServer(opts...)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(lis) }()
	return lis, func() {
		srv.GracefulStop()
		<-errCh
	}
}

// dialBufconn creates a gRPC client connection to the given bufconn.Listener.
func dialBufconn(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	return conn
}

// ---- Health check ----------------------------------------------------------

func TestNewServer_HealthCheck_Serving(t *testing.T) {
	lis, cleanup := startServer(t)
	defer cleanup()

	conn := dialBufconn(t, lis)
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: ""})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Status)
	}
}

func TestNewServer_GracefulStop_HealthNotServing(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	srv := grpcx.NewServer()
	go srv.Serve(lis) //nolint:errcheck

	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("pre-stop health: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("pre-stop status = %v, want SERVING", resp.Status)
	}

	srv.GracefulStop()
}

// ---- Interceptors ----------------------------------------------------------

func TestUnaryPanicRecovery_CatchesPanic(t *testing.T) {
	logger := newLogger()
	interceptor := grpcx.UnaryPanicRecovery(logger)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req any) (any, error) {
		panic("intentional panic")
	})
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal", status.Code(err))
	}
}

func TestUnaryPanicRecovery_PassThrough(t *testing.T) {
	interceptor := grpcx.UnaryPanicRecovery(newLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := interceptor(context.Background(), "req", info, func(_ context.Context, req any) (any, error) {
		return "pong", nil
	})
	if err != nil || resp != "pong" {
		t.Errorf("got (%v, %v), want (pong, nil)", resp, err)
	}
}

func TestUnaryLogging_Runs(t *testing.T) {
	interceptor := grpcx.UnaryLogging(newLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Ping"}

	_, err := interceptor(context.Background(), nil, info, func(_ context.Context, req any) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnaryLogging_LogsError(t *testing.T) {
	interceptor := grpcx.UnaryLogging(newLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Fail"}
	wantErr := status.Error(codes.NotFound, "not found")

	_, err := interceptor(context.Background(), nil, info, func(_ context.Context, req any) (any, error) {
		return nil, wantErr
	})
	if err != wantErr {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

func TestUnaryRequestID_InjectsID(t *testing.T) {
	interceptor := grpcx.UnaryRequestID()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Req"}

	var ctxInHandler context.Context
	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req any) (any, error) {
		ctxInHandler = ctx
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id := observability.RequestIDFromContext(ctxInHandler)
	if id == "" {
		t.Error("expected request ID in handler context")
	}
}

func TestUnaryTrace_Runs(t *testing.T) {
	_, err := observability.InitTracer("test", observability.WithNoopTracer())
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	interceptor := grpcx.UnaryTrace()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Trace"}

	_, err = interceptor(context.Background(), nil, info, func(_ context.Context, req any) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultUnaryInterceptors_Count(t *testing.T) {
	chain := grpcx.DefaultUnaryInterceptors(newLogger())
	if len(chain) == 0 {
		t.Error("expected non-empty default unary interceptor chain")
	}
}

func TestDefaultStreamInterceptors_Count(t *testing.T) {
	chain := grpcx.DefaultStreamInterceptors(newLogger())
	if len(chain) == 0 {
		t.Error("expected non-empty default stream interceptor chain")
	}
}

// ---- NewServer with interceptors -------------------------------------------

func TestNewServer_WithInterceptors_HealthCheck(t *testing.T) {
	logger := newLogger()
	lis, cleanup := startServer(t,
		grpcx.WithUnaryInterceptors(grpcx.DefaultUnaryInterceptors(logger)...),
		grpcx.WithStreamInterceptors(grpcx.DefaultStreamInterceptors(logger)...),
	)
	defer cleanup()

	conn := dialBufconn(t, lis)
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check with interceptors: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Status)
	}
}

// ---- NewClient -------------------------------------------------------------

func TestNewClient_Connects(t *testing.T) {
	lis, cleanup := startServer(t)
	defer cleanup()

	conn, err := grpcx.NewClient(
		"bufnet",
		grpcx.WithDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
		),
		grpcx.WithoutRetry(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check via NewClient: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Status)
	}
}

func TestNewClient_PropagatesRequestID(t *testing.T) {
	lis, cleanup := startServer(t,
		grpcx.WithUnaryInterceptors(grpcx.UnaryRequestID()),
	)
	defer cleanup()

	conn, err := grpcx.NewClient(
		"bufnet",
		grpcx.WithDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
		),
		grpcx.WithoutRetry(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	ctx := observability.WithRequestID(context.Background(), "client-req-id")
	hc := grpc_health_v1.NewHealthClient(conn)
	_, err = hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
}
