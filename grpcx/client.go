package grpcx

import (
	"context"

	"github.com/harnamsingh/go-servicekit/observability"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Dial creates a new gRPC client connection to target using insecure
// transport by default. Pass additional DialOptions to override credentials or
// add interceptors. The connection is established lazily on the first RPC call.
func Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	defaults := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(propagateRequestIDUnary),
		grpc.WithChainStreamInterceptor(propagateRequestIDStream),
	}
	return grpc.NewClient(target, append(defaults, opts...)...)
}

// propagateRequestIDUnary copies the request ID from the outgoing context into
// gRPC metadata so the backend receives it.
func propagateRequestIDUnary(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	ctx = appendRequestIDMD(ctx)
	return invoker(ctx, method, req, reply, cc, opts...)
}

// propagateRequestIDStream does the same for streaming calls.
func propagateRequestIDStream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	ctx = appendRequestIDMD(ctx)
	return streamer(ctx, desc, cc, method, opts...)
}

func appendRequestIDMD(ctx context.Context) context.Context {
	if rid := observability.RequestIDFromContext(ctx); rid != "" {
		return metadata.AppendToOutgoingContext(ctx, headerRequestID, rid)
	}
	return ctx
}
