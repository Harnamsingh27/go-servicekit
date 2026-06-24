package grpcx

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/harnamsingh/go-servicekit/observability"
)

// wrappedStream overrides Context so interceptors can inject values.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// UnaryPanicRecovery catches panics in unary RPC handlers, logs the stack
// trace, and returns codes.Internal to the caller.
func UnaryPanicRecovery(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorContext(ctx, "panic in unary handler",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamPanicRecovery catches panics in stream RPC handlers.
func StreamPanicRecovery(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorContext(ss.Context(), "panic in stream handler",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// UnaryLogging records a structured log line for each unary RPC with the
// method name, status code, and latency.
func UnaryLogging(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)
		logger.InfoContext(ctx, "grpc unary",
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.String("latency", time.Since(start).String()),
		)
		return resp, err
	}
}

// StreamLogging records a log line when a streaming RPC completes.
func StreamLogging(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		code := status.Code(err)
		logger.InfoContext(ss.Context(), "grpc stream",
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.String("latency", time.Since(start).String()),
		)
		return err
	}
}

// UnaryRequestID extracts the request ID from incoming gRPC metadata (or
// generates a new one) and stores it in the context.
func UnaryRequestID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = observability.WithRequestIDFromMetadata(ctx)
		return handler(ctx, req)
	}
}

// StreamRequestID injects a request ID into each streaming RPC context.
func StreamRequestID() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := observability.WithRequestIDFromMetadata(ss.Context())
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// UnaryTrace starts an OpenTelemetry span for each unary RPC.
func UnaryTrace() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tracer := otel.Tracer("grpcx")
		ctx, span := tracer.Start(ctx, info.FullMethod)
		defer span.End()
		resp, err := handler(ctx, req)
		if err != nil {
			span.SetStatus(otelcodes.Error, err.Error())
		}
		return resp, err
	}
}

// StreamTrace starts an OpenTelemetry span for each streaming RPC.
func StreamTrace() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tracer := otel.Tracer("grpcx")
		ctx, span := tracer.Start(ss.Context(), info.FullMethod)
		defer span.End()
		err := handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
		if err != nil {
			span.SetStatus(otelcodes.Error, err.Error())
		}
		return err
	}
}

// DefaultUnaryInterceptors returns the standard unary interceptor chain:
//
//	UnaryPanicRecovery -> UnaryRequestID -> UnaryLogging -> UnaryTrace
func DefaultUnaryInterceptors(logger *slog.Logger) []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		UnaryPanicRecovery(logger),
		UnaryRequestID(),
		UnaryLogging(logger),
		UnaryTrace(),
	}
}

// DefaultStreamInterceptors returns the standard stream interceptor chain.
func DefaultStreamInterceptors(logger *slog.Logger) []grpc.StreamServerInterceptor {
	return []grpc.StreamServerInterceptor{
		StreamPanicRecovery(logger),
		StreamRequestID(),
		StreamLogging(logger),
		StreamTrace(),
	}
}

// UnaryClientRequestID propagates the request ID from context to outgoing metadata.
func UnaryClientRequestID() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := observability.RequestIDFromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, observability.RequestIDMetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
