package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// setupTracer configures OpenTelemetry tracing for the given service name.
func setupTracer(service string) (func(context.Context) error, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	r, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", service),
	))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(r),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// main demonstrates OpenTelemetry tracing middleware in flash.
func main() {
	shutdown, err := setupTracer("demo-service")
	if err != nil {
		log.Fatalf("setup tracer: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	app := flash.New()

	// Group 1: simple OTel usage
	simple := app.Group("/simple", mw.OTel("demo-service"))
	simple.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "simple ok") })
	simple.GET("/slow", func(c *flash.Ctx) error {
		time.Sleep(50 * time.Millisecond)
		return c.String(http.StatusOK, "simple slow")
	})

	// Group 2: advanced OTel config
	configGroup := app.Group("/config", mw.OTelWithConfig(mw.OTelConfig{
		ServiceName:    "demo-service-config",
		RecordDuration: true,
		SpanName: func(c *flash.Ctx) string {
			if rt := c.Route(); rt != "" {
				return c.Method() + " " + rt
			}
			return c.Method() + " " + c.Path()
		},
		Attributes: func(c *flash.Ctx) []attribute.KeyValue {
			return []attribute.KeyValue{
				attribute.String("client.addr", c.Request().RemoteAddr),
			}
		},
		Status: func(code int, err error) (codes.Code, string) {
			// Example: mark 4xx as Error with custom message; 5xx or errors as Error
			if code >= 400 && code < 500 {
				return codes.Error, "client error"
			}
			if err != nil || code >= 500 {
				return codes.Error, http.StatusText(code)
			}
			return codes.Ok, ""
		},
		ExtraAttributes: []attribute.KeyValue{
			attribute.String("env", "dev"),
		},
	}))
	configGroup.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "config ok") })
	configGroup.GET("/bad", func(c *flash.Ctx) error { return c.String(http.StatusBadRequest, "config bad") })

	log.Fatal(http.ListenAndServe(":8080", app))
}
