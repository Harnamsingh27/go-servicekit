package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TraceOptions configures the OTel tracer provider.
type TraceOptions struct {
	ServiceName    string
	ServiceVersion string
	// OTLPEndpoint is the gRPC endpoint of an OTLP-compatible collector
	// (e.g. "localhost:4317"). When empty a no-op tracer is installed.
	OTLPEndpoint string
}

// SetupTracer initialises the global OTel tracer provider and text-map
// propagator. When opts.OTLPEndpoint is empty a no-op provider is installed so
// callers never need to nil-check the tracer. The returned function must be
// called to flush pending spans and shut down the exporter.
func SetupTracer(ctx context.Context, opts TraceOptions) (shutdown func(context.Context) error, err error) {
	if opts.OTLPEndpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(opts.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create OTLP trace exporter: %w", err)
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(opts.ServiceName),
			semconv.ServiceVersion(opts.ServiceVersion),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns a named tracer from the global OTel provider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
