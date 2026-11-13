package tracer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer configures the global OpenTelemetry TracerProvider with standard W3C TraceContext propagation.
func InitTracer(_ context.Context, serviceName, environment string) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String(serviceName),
			semconv.DeploymentEnvironmentKey.String(environment),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	cleanup := func(shutdownCtx context.Context) error {
		return tp.Shutdown(shutdownCtx)
	}

	return tp, cleanup, nil
}
