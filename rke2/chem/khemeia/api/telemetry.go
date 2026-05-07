package main

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func initTracer(ctx context.Context) (func(), error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "tempo.monitor:4317"
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return func() {}, err
	}
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return func() {}, err
	}
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("khemeia-controller"),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return func() { _ = tp.Shutdown(ctx) }, nil
}

func initMetrics() error {
	exp, err := prometheus.New()
	if err != nil {
		return err
	}
	provider := metric.NewMeterProvider(metric.WithReader(exp))
	otel.SetMeterProvider(provider)
	return nil
}
