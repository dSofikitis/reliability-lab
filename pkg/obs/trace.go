// Tracing setup. Each service calls InitTracing once at startup and
// defers the returned shutdown. The exporter is OTLP/gRPC, pointed at
// the OTel Collector running in-cluster (k8s/observability/otel-collector.yaml).
//
// Endpoint is configured via the standard OTEL_EXPORTER_OTLP_ENDPOINT
// env var so anyone running locally without a collector can just unset
// the var and tracing becomes a no-op (the SDK still starts but nothing
// leaves the process).
package obs

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracing configures a global TracerProvider for the calling service
// and returns a shutdown func the caller must defer. If the OTLP endpoint
// is unset the provider is still installed (so tracer handles work), but
// span data is dropped at the exporter — no crash on missing collector.
func InitTracing(ctx context.Context, service, version string) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(service),
			semconv.ServiceVersion(version),
		),
		resource.WithProcess(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithInsecure()}
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(stripScheme(ep)))
	}
	exp, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio()))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns a named tracer from the globally-installed provider.
// Library convention: pass the package's import path as name.
func Tracer(name string) trace.Tracer { return otel.Tracer(name) }

func stripScheme(ep string) string {
	for _, p := range []string{"http://", "https://", "grpc://"} {
		if len(ep) > len(p) && ep[:len(p)] == p {
			return ep[len(p):]
		}
	}
	return ep
}

func samplingRatio() float64 {
	v := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if v == "" {
		return 1.0
	}
	var r float64
	if _, err := fmt.Sscanf(v, "%f", &r); err != nil || r <= 0 {
		return 1.0
	}
	if r > 1 {
		return 1
	}
	return r
}
