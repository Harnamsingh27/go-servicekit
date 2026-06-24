package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/harnamsingh/go-servicekit/grpcx"
	"github.com/harnamsingh/go-servicekit/observability"
)

func main() {
	logger := observability.NewLogger()

	_, err := observability.InitTracer("grpc-demo", observability.WithNoopTracer())
	if err != nil {
		logger.Error("init tracer", slog.Any("err", err))
		os.Exit(1)
	}

	srv := grpcx.NewServer(
		grpcx.WithUnaryInterceptors(grpcx.DefaultUnaryInterceptors(logger)...),
		grpcx.WithStreamInterceptors(grpcx.DefaultStreamInterceptors(logger)...),
	)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("listen", slog.Any("err", err))
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("grpc-demo listening", slog.String("addr", ":50051"))
		if err := srv.Serve(lis); err != nil {
			logger.Error("serve", slog.Any("err", err))
		}
	}()

	<-stop
	logger.Info("shutting down")
	srv.GracefulStop()

	// Demonstrate the client: dial and check health.
	conn, err := grpcx.NewClient(":50051")
	if err != nil {
		logger.Warn("client dial skipped (server already stopped)", slog.Any("err", err))
		return
	}
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		logger.Warn("health check after stop", slog.Any("err", err))
		return
	}
	logger.Info("health", slog.String("status", resp.Status.String()))
}
