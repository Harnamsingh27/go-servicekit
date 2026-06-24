package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// JWTConfig holds the settings used to verify incoming tokens.
type JWTConfig struct {
	// Secret is the HMAC-SHA256 key used to sign and verify HS256 tokens.
	Secret []byte
}

// ValidateToken parses and validates rawToken, returning the extracted Claims.
func (c JWTConfig) ValidateToken(rawToken string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(rawToken, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		return c.Secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// HTTPMiddleware returns an http.Handler that validates a Bearer token from the
// Authorization header. Valid claims are stored in the request context via
// WithClaims; missing or invalid tokens produce a 401 response.
func (c JWTConfig) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r.Header.Get("Authorization"))
		if raw == "" {
			http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
			return
		}
		claims, err := c.ValidateToken(raw)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// UnaryInterceptor is a gRPC unary server interceptor that validates a Bearer
// token from the incoming "authorization" metadata key.
func (c JWTConfig) UnaryInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md["authorization"]
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
	}
	raw := bearerToken(vals[0])
	if raw == "" {
		return nil, status.Error(codes.Unauthenticated, "malformed authorization metadata")
	}
	claims, err := c.ValidateToken(raw)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}
	return handler(WithClaims(ctx, claims), req)
}

// StreamInterceptor is a gRPC stream server interceptor that validates a Bearer
// token from the incoming "authorization" metadata key.
func (c JWTConfig) StreamInterceptor(
	srv any,
	ss grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md["authorization"]
	if len(vals) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization metadata")
	}
	raw := bearerToken(vals[0])
	if raw == "" {
		return status.Error(codes.Unauthenticated, "malformed authorization metadata")
	}
	claims, err := c.ValidateToken(raw)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}
	return handler(srv, &wrappedStream{ServerStream: ss, ctx: WithClaims(ss.Context(), claims)})
}

// wrappedStream overrides the context on an existing ServerStream.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// bearerToken extracts the token from a "Bearer <token>" header value.
func bearerToken(h string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}
