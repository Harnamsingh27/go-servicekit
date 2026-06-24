package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// MetricOptions configures the OTel meter provider.
type MetricOptions struct {
	// OTLPEndpoint is the gRPC endpoint of an OTLP-compatible collector.
	// When empty a no-op meter is installed.
	OTLPEndpoint string
}

// SetupMetrics initialises the global OTel meter provider. When
// opts.OTLPEndpoint is empty a no-op provider is installed. The returned
// function must be called to flush and shut down the exporter.
func SetupMetrics(ctx context.Context, opts MetricOptions) (shutdown func(context.Context) error, err error) {
	if opts.OTLPEndpoint == "" {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(opts.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create OTLP metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
	)
	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}

// Meter returns a named meter from the global OTel provider.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}
