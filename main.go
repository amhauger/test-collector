package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func tracingMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		tracer := otel.Tracer("test-collector.monitoring.middleware")
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := tracer.Start(ctx, fmt.Sprintf("test-collector.%s", c.Request.RequestURI))

		c.Set("OtelTraceContext", ctx)

		defer span.End()

		span.SetAttributes(
			attribute.String("http.route", c.Request.RequestURI),
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.path", c.Request.URL.Path),
		)

		writer := &responseBodyWriter{
			body:           bytes.NewBufferString(""),
			ResponseWriter: c.Writer,
		}
		c.Writer = writer
		c.Next()

		span.SetAttributes(
			attribute.String("http.response.body", writer.body.String()),
			attribute.Int("http.status_code", writer.Status()),
		)
	}
}

func setupOtel() (func(), error) {
	ctx := context.Background()
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team": "feJIrcsnDM5Qyzjbxe2nra",
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

	g := gin.New()
	g.Use(tracingMiddleware())
	g.GET("/health", healthcheck)

	log.Fatal().Err(g.Run(":8080"))
}

func healthcheck(c *gin.Context) {
	traceContext := c.MustGet("OtelTraceContext")
	_, span := otel.Tracer("test-collector.main").Start(traceContext.(context.Context), "app.healthcheck")
	defer span.End()
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
}
