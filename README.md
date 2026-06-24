# go-servicekit

A production-grade, inner-source Go standard library for building HTTP and gRPC services at scale.

[![CI](https://github.com/harnamsingh/go-servicekit/actions/workflows/ci.yml/badge.svg)](https://github.com/harnamsingh/go-servicekit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/harnamsingh/go-servicekit.svg)](https://pkg.go.dev/github.com/harnamsingh/go-servicekit)
[![Go Version](https://img.shields.io/badge/go-1.22+-blue)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Why this exists

Every new Go service ends up copying the same boilerplate: structured logging wired to request IDs, JWT middleware, config loading from multiple sources, panic recovery, graceful shutdown. `go-servicekit` captures those patterns once, tests them thoroughly, and exposes them through a minimal, composable API so teams can focus on domain logic instead of infrastructure plumbing.

---

## Packages

| Package | Purpose |
|---|---|
| [`errors`](errors/) | Typed `AppError` with HTTP + gRPC status mapping |
| [`config`](config/) | Generic multi-source loader (YAML, .env, env vars, overrides) |
| [`observability`](observability/) | Structured logging, OpenTelemetry tracing + metrics, request-ID propagation |
| [`auth`](auth/) | JWT verification, API-key middleware, gRPC interceptors |
| [`httpx`](httpx/) | HTTP server with production defaults, middleware chain, retry client |
| [`grpcx`](grpcx/) | gRPC server with interceptors, health check, retry client |

---

## Quickstart

```go
import (
    "github.com/harnamsingh/go-servicekit/httpx"
    "github.com/harnamsingh/go-servicekit/observability"
)

logger := observability.NewLogger()
mws    := httpx.DefaultMiddleware(logger, 10*time.Second)

srv := httpx.NewServer(
    httpx.WithAddr(":8080"),
    httpx.WithServerMiddleware(mws...),
)
srv.Router().GET("/hello", helloHandler)
srv.ListenAndServe()
```

See [`examples/http-demo`](examples/http-demo/) and [`examples/grpc-demo`](examples/grpc-demo/) for full runnable programs.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        go-servicekit                        │
│                                                             │
│  ┌─────────┐  ┌────────┐  ┌───────────────┐  ┌──────────┐ │
│  │ errors  │  │ config │  │ observability │  │   auth   │ │
│  └────┬────┘  └────────┘  └───────┬───────┘  └────┬─────┘ │
│       │                           │                │        │
│  ┌────┴───────────────────────────┴────────────────┴─────┐ │
│  │               httpx                grpcx              │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

`errors` and `observability` are foundational; `httpx` and `grpcx` build on all layers.

---

## Installation

```bash
go get github.com/harnamsingh/go-servicekit
```

Requires **Go 1.22+**.

---

## errors

```go
import "github.com/harnamsingh/go-servicekit/errors"

err := errors.New(errors.CodeNotFound, "user not found")
http.Error(w, err.Error(), errors.ToHTTPStatus(err)) // 404

wrapped := errors.Wrap(errors.CodeInternal, "db query failed", sqlErr)
errors.ToGRPCStatus(wrapped).Err() // codes.Internal
```

---

## config

```go
import "github.com/harnamsingh/go-servicekit/config"

type ServerConfig struct {
    Host   string        `yaml:"host"   env:"HOST"   default:"localhost"`
    Port   int           `yaml:"port"   env:"PORT"   default:"8080"      validate:"required"`
    Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" default:"30s"`
}

cfg, err := config.Load[ServerConfig](
    config.WithYAMLFile("config.yaml"),
    config.WithEnvFile(".env"),
)
```

Sources are merged in ascending precedence: struct defaults → YAML → .env → env vars → explicit overrides.

---

## observability

```go
import "github.com/harnamsingh/go-servicekit/observability"

// Logger (log/slog, JSON, auto-injects request_id + trace_id from context)
logger := observability.NewLogger()

// Tracer (OpenTelemetry, OTLP gRPC or stdout)
shutdown, _ := observability.InitTracer("my-service",
    observability.WithOTLPEndpoint("otelcol:4317"),
)
defer shutdown(ctx)

// Metrics (OpenTelemetry)
shutdown, _ = observability.InitMetrics("my-service",
    observability.WithStdoutMetrics(),
)

// Request-ID middleware (HTTP)
http.Handle("/", observability.RequestIDMiddleware(myHandler))
```

---

## auth

```go
import "github.com/harnamsingh/go-servicekit/auth"

verifier := auth.NewHMACVerifier([]byte(os.Getenv("JWT_SECRET")),
    auth.WithIssuer("my-service"),
)

// HTTP
mux.Handle("/api/", auth.JWTMiddleware(verifier)(apiRouter))

// gRPC
grpcx.NewServer(
    grpcx.WithUnaryInterceptors(auth.JWTUnaryInterceptor(verifier)),
)

// API key
store := auth.NewMemoryKeyStore("key-1", "key-2")
mux.Handle("/webhook", auth.APIKeyMiddleware(store)(webhookHandler))
```

---

## httpx

```go
import "github.com/harnamsingh/go-servicekit/httpx"

srv := httpx.NewServer(
    httpx.WithAddr(":8080"),
    httpx.WithReadTimeout(5 * time.Second),
    httpx.WithServerMiddleware(httpx.DefaultMiddleware(logger, 30*time.Second)...),
)
r := srv.Router()
r.GET("/health", healthHandler)
v1 := r.Group("/v1")
v1.POST("/users", createUserHandler)

// Client with retries
client := httpx.NewClient(httpx.WithRetry(3, 100*time.Millisecond))
resp, err := client.Get(ctx, "https://api.example.com/resource")
```

---

## grpcx

```go
import "github.com/harnamsingh/go-servicekit/grpcx"

srv := grpcx.NewServer(
    grpcx.WithUnaryInterceptors(grpcx.DefaultUnaryInterceptors(logger)...),
    grpcx.WithStreamInterceptors(grpcx.DefaultStreamInterceptors(logger)...),
)
mypb.RegisterMyServiceServer(srv.RegisterService, &myServiceImpl{})
srv.Serve(lis)
defer srv.GracefulStop()

// Client
conn, err := grpcx.NewClient("localhost:50051")
```

---

## Development

```bash
make lint        # golangci-lint
make test        # go test ./...
make test-race   # go test -race ./...
make cover       # HTML coverage report
make vet         # go vet ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full inner-source workflow.

---

## License

MIT — see [LICENSE](LICENSE).