# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

---

## [0.1.0] - 2026-06-24

### Added

- **`errors`** — typed `AppError` with `ErrorCode` constants, `New`, `Newf`, `Wrap` constructors, `ToHTTPStatus`, and `ToGRPCStatus` mappers.
- **`config`** — generic `Load[T]` function with five-source merge (struct defaults, YAML file, .env file, environment variables, explicit overrides) and optional `Validator` hook.
- **`observability`**
  - `NewLogger` — structured JSON logger backed by `log/slog` with a context handler that auto-injects `request_id`, `trace_id`, and `span_id`.
  - `InitTracer` — OpenTelemetry `TracerProvider` with OTLP gRPC, stdout, and noop exporters.
  - `InitMetrics` — OpenTelemetry `MeterProvider` with OTLP gRPC and stdout exporters; `NewHTTPMetrics` + `HTTPMetricsMiddleware`.
  - Request-ID helpers: `NewRequestID`, `WithRequestID`, `RequestIDFromContext`, `RequestIDMiddleware`, and gRPC metadata variants.
- **`auth`**
  - `HMACVerifier` — HMAC-signed JWT verification with optional issuer and audience validation.
  - `JWTMiddleware` — HTTP Bearer token middleware; `JWTUnaryInterceptor` / `JWTStreamInterceptor` for gRPC.
  - `MemoryKeyStore` / `APIKeyMiddleware` — in-memory API key store with runtime add/remove.
- **`httpx`**
  - `NewServer` — `net/http` server with production defaults and functional options.
  - Middleware: `PanicRecoveryMiddleware`, `AccessLogMiddleware`, `TimeoutMiddleware`, `DefaultMiddleware`.
  - `Router` — thin `http.ServeMux` wrapper with method helpers and `Group` prefix support.
  - `Client` — HTTP client with exponential-backoff retries, per-request timeout, and request-ID propagation.
- **`grpcx`**
  - `NewServer` — gRPC server with automatic health service registration.
  - Interceptors: `UnaryPanicRecovery`, `StreamPanicRecovery`, `UnaryLogging`, `StreamLogging`, `UnaryRequestID`, `StreamRequestID`, `UnaryTrace`, `StreamTrace`.
  - `NewClient` — gRPC client with built-in retry policy and request-ID propagation.
- **`examples/http-demo`** — runnable HTTP service demonstrating the full stack.
- **`examples/grpc-demo`** — runnable gRPC service demonstrating the full interceptor chain.
- Makefile with `lint`, `test`, `test-race`, `cover`, `vet`, `build`, `clean` targets.
- CI workflow (GitHub Actions) running on Go 1.22 and 1.23.

[Unreleased]: https://github.com/harnamsingh/go-servicekit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/harnamsingh/go-servicekit/releases/tag/v0.1.0