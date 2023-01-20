package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

func setupOtel() (func(), error) {
	ctx := context.Background()
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team": "zewOz9u27ik8uAW12ER8PA",
		}),
		otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
	)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, err
	}

	tracer := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes("", attribute.KeyValue{
			Key:   "service.name",
			Value: attribute.StringValue("test-collector"),
		})),
	)

	otel.SetTracerProvider(tracer)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func() { _ = tracer.Shutdown(ctx) }, nil
}

func main() {
	shutdownFunc, err := setupOtel()
	if err != nil {
		panic(err)
	}
	defer shutdownFunc()

	tracer := otel.Tracer("test-collector.main")
	_, mainSpan := tracer.Start(
		context.Background(),
		"app.main.main",
	)
	mainSpan.End()
}
