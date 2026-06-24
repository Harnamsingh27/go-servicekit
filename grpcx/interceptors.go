package grpcx

import (
	"context"

	"go-servicekit/observability"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const headerRequestID = "x-request-id"

// requestIDUnaryInterceptor extracts an incoming x-request-id metadata header
// (if present) and stores it in the context, or generates a new one.
func requestIDUnaryInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	ctx = injectRequestID(ctx)
	return handler(ctx, req)
}

// requestIDStreamInterceptor does the same for streaming RPCs.
func requestIDStreamInterceptor(
	srv any,
	ss grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	ctx := injectRequestID(ss.Context())
	return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
}

func injectRequestID(ctx context.Context) context.Context {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(headerRequestID); len(vals) > 0 {
			return observability.WithRequestID(ctx, vals[0])
		}
	}
	return observability.WithRequestID(ctx, observability.NewRequestID())
}

// wrappedStream overrides the context on an existing grpc.ServerStream.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
