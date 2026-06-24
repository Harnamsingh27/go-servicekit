package grpcx

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultDialTimeout = 5 * time.Second
	defaultRetryPolicy = `{
		"methodConfig": [{
			"name": [{}],
			"retryPolicy": {
				"maxAttempts": 4,
				"initialBackoff": "0.1s",
				"maxBackoff": "2s",
				"backoffMultiplier": 2,
				"retryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED"]
			}
		}]
	}`
)

// ClientOption configures NewClient.
type ClientOption func(*clientConfig)

type clientConfig struct {
	dialTimeout        time.Duration
	disableRetry       bool
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
	dialOpts           []grpc.DialOption
}

// WithDialTimeout sets the maximum time to wait for the connection to be established.
func WithDialTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.dialTimeout = d }
}

// WithClientUnaryInterceptors appends unary client interceptors.
func WithClientUnaryInterceptors(interceptors ...grpc.UnaryClientInterceptor) ClientOption {
	return func(c *clientConfig) {
		c.unaryInterceptors = append(c.unaryInterceptors, interceptors...)
	}
}

// WithoutRetry disables the default retry service config.
func WithoutRetry() ClientOption {
	return func(c *clientConfig) { c.disableRetry = true }
}

// WithDialOptions appends raw grpc.DialOption values.
func WithDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(c *clientConfig) { c.dialOpts = append(c.dialOpts, opts...) }
}

// NewClient dials target and returns a *grpc.ClientConn with sensible defaults:
// insecure transport, built-in retry policy, and request-ID propagation.
// Call conn.Close() when done.
func NewClient(target string, opts ...ClientOption) (*grpc.ClientConn, error) {
	cfg := &clientConfig{
		dialTimeout: defaultDialTimeout,
	}
	for _, o := range opts {
		o(cfg)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if !cfg.disableRetry {
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(defaultRetryPolicy))
	}

	unary := append([]grpc.UnaryClientInterceptor{UnaryClientRequestID()}, cfg.unaryInterceptors...)
	dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(unary...))

	if len(cfg.streamInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainStreamInterceptor(cfg.streamInterceptors...))
	}

	dialOpts = append(dialOpts, cfg.dialOpts...)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, dialOpts...) //nolint:staticcheck
	if err != nil {
		return nil, fmt.Errorf("grpcx: dial %q: %w", target, err)
	}
	return conn, nil
}
