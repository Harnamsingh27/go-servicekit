package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harnamsingh/go-servicekit/observability"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/metadata"
)

// ---- Logger tests ----------------------------------------------------------

func TestNewLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(observability.WithLogOutput(&buf))
	log.Info("hello", "key", "value")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("log output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if m["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", m["msg"])
	}
}

func TestNewLogger_AttachesRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(observability.WithLogOutput(&buf))

	ctx := observability.WithRequestID(context.Background(), "test-req-id")
	log.InfoContext(ctx, "with request id")

	if !strings.Contains(buf.String(), "test-req-id") {
		t.Errorf("log output does not contain request_id; got: %s", buf.String())
	}
}

func TestNewLogger_AttachesTraceID(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(observability.WithLogOutput(&buf))

	// Set up an in-memory tracer.
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)))
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	ctx, span := otel.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	log.InfoContext(ctx, "with trace")

	if !strings.Contains(buf.String(), "trace_id") {
		t.Errorf("log output does not contain trace_id; got: %s", buf.String())
	}
}

// ---- Tracer tests ----------------------------------------------------------

func TestInitTracer_Noop(t *testing.T) {
	shutdown, err := observability.InitTracer("test-svc", observability.WithNoopTracer())
	if err != nil {
		t.Fatalf("InitTracer noop: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestInitTracer_Stdout(t *testing.T) {
	var buf bytes.Buffer
	shutdown, err := observability.InitTracer("test-svc",
		observability.WithStdoutTracer(&buf),
	)
	if err != nil {
		t.Fatalf("InitTracer stdout: %v", err)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	ctx, span := observability.Tracer("test").Start(context.Background(), "op")
	span.End()
	shutdown(context.Background()) //nolint:errcheck

	_ = ctx
	// Stdout tracer may buffer; just verify no panic and no error.
}

func TestInitTracer_NoExporter(t *testing.T) {
	_, err := observability.InitTracer("test-svc")
	if err == nil {
		t.Error("expected error when no exporter is configured")
	}
}

func TestInitTracer_SpanContextPropagation(t *testing.T) {
	// Use in-memory exporter to assert spans are created.
	exp := tracetest.NewInMemoryExporter()
	res, _ := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName("prop-test")),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, child := otel.Tracer("test").Start(ctx, "child")
	child.End()
	parent.End()

	spans := exp.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d", len(spans))
	}
	// Child span's parent should be the parent span.
	var childSpan tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "child" {
			childSpan = s
		}
	}
	if !childSpan.Parent.IsValid() {
		t.Error("child span has no valid parent span context")
	}
}

// ---- RequestID tests -------------------------------------------------------

func TestNewRequestID(t *testing.T) {
	id := observability.NewRequestID()
	if len(id) != 36 {
		t.Errorf("NewRequestID() len = %d, want 36 (UUID)", len(id))
	}
	id2 := observability.NewRequestID()
	if id == id2 {
		t.Error("consecutive IDs should not be equal")
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := observability.WithRequestID(context.Background(), "abc-123")
	got := observability.RequestIDFromContext(ctx)
	if got != "abc-123" {
		t.Errorf("RequestIDFromContext = %q, want abc-123", got)
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	if id := observability.RequestIDFromContext(context.Background()); id != "" {
		t.Errorf("expected empty ID from bare context, got %q", id)
	}
}

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	called := false
	h := observability.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		id := observability.RequestIDFromContext(r.Context())
		if id == "" {
			t.Error("request ID missing from context")
		}
		if w.Header().Get(observability.RequestIDHeader) != id {
			t.Error("X-Request-ID response header mismatch")
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if !called {
		t.Error("handler was not called")
	}
}

func TestRequestIDMiddleware_PropagatesExistingID(t *testing.T) {
	const wantID = "my-existing-id"
	h := observability.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := observability.RequestIDFromContext(r.Context())
		if got != wantID {
			t.Errorf("request ID = %q, want %q", got, wantID)
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(observability.RequestIDHeader, wantID)
	h.ServeHTTP(rec, req)
}

func TestRequestIDFromMetadata(t *testing.T) {
	md := metadata.Pairs(observability.RequestIDMetadataKey, "grpc-req-id")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := observability.RequestIDFromMetadata(ctx)
	if got != "grpc-req-id" {
		t.Errorf("RequestIDFromMetadata = %q, want grpc-req-id", got)
	}
}

func TestWithRequestIDFromMetadata_GeneratesIfAbsent(t *testing.T) {
	ctx := observability.WithRequestIDFromMetadata(context.Background())
	id := observability.RequestIDFromContext(ctx)
	if id == "" {
		t.Error("expected a generated request ID, got empty string")
	}
}

func TestInjectRequestIDToMetadata(t *testing.T) {
	ctx := observability.WithRequestID(context.Background(), "inject-me")
	ctx = observability.InjectRequestIDToMetadata(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("no outgoing metadata found")
	}
	vals := md.Get(observability.RequestIDMetadataKey)
	if len(vals) == 0 || vals[0] != "inject-me" {
		t.Errorf("outgoing metadata x-request-id = %v, want [inject-me]", vals)
	}
}

func TestInjectRequestIDToMetadata_NoID(t *testing.T) {
	ctx := observability.InjectRequestIDToMetadata(context.Background())
	_, ok := metadata.FromOutgoingContext(ctx)
	_ = ok // should not panic; no metadata injected
}

// ---- Tracer stdout exporter (verify spans recorded) -----------------------

func TestInitTracer_StdoutSpansWritten(t *testing.T) {
	var buf bytes.Buffer
	exp, err := stdouttrace.New(stdouttrace.WithWriter(&buf), stdouttrace.WithPrettyPrint())
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	_, span := otel.Tracer("test").Start(context.Background(), "my-op")
	span.End()
	tp.Shutdown(context.Background()) //nolint:errcheck

	if !strings.Contains(buf.String(), "my-op") {
		t.Errorf("expected span name in stdout output; got: %s", buf.String())
	}
}

// ---- Logger option tests ---------------------------------------------------

func TestWithLogLevel_Debug(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(
		observability.WithLogOutput(&buf),
		observability.WithLogLevel(-4), // slog.LevelDebug
	)
	log.Debug("debug msg")
	if !strings.Contains(buf.String(), "debug msg") {
		t.Error("DEBUG record not emitted at LevelDebug")
	}
}

func TestWithLogSource(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(
		observability.WithLogOutput(&buf),
		observability.WithLogSource(),
	)
	log.Info("source test")
	if !strings.Contains(buf.String(), "source") {
		t.Error("source field missing from log record with WithLogSource")
	}
}

func TestContextHandler_WithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(observability.WithLogOutput(&buf))
	// WithAttrs
	log = log.With("persistent_key", "pval")
	log.Info("with attrs")
	if !strings.Contains(buf.String(), "persistent_key") {
		t.Error("WithAttrs: persistent_key missing")
	}
	// WithGroup
	buf.Reset()
	log = log.WithGroup("grp")
	log.Info("with group", "k", "v")
	if !strings.Contains(buf.String(), "grp") {
		t.Error("WithGroup: group name missing")
	}
}

// ---- Metrics tests ---------------------------------------------------------

func TestInitMetrics_Stdout(t *testing.T) {
	shutdown, err := observability.InitMetrics("test-svc", observability.WithStdoutMetrics())
	if err != nil {
		t.Fatalf("InitMetrics stdout: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestInitMetrics_NoExporter(t *testing.T) {
	_, err := observability.InitMetrics("test-svc")
	if err == nil {
		t.Error("expected error when no metric exporter configured")
	}
}

func TestNewHTTPMetrics_AndMiddleware(t *testing.T) {
	_, err := observability.InitMetrics("test-svc", observability.WithStdoutMetrics())
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	m := observability.Meter("test")
	metrics, err := observability.NewHTTPMetrics(m)
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	mw := observability.HTTPMetricsMiddleware(metrics)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(rec, req) // should not panic
}

func TestHTTPMetricsMiddleware_CapturesStatus(t *testing.T) {
	_, err := observability.InitMetrics("test-svc2", observability.WithStdoutMetrics())
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	m := observability.Meter("test2")
	metrics, err := observability.NewHTTPMetrics(m)
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	mw := observability.HTTPMetricsMiddleware(metrics)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
