package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestOTelMiddlewareDoesNotBlock(t *testing.T) {
	a := flash.New()
	a.Use(OTel("test-svc"))
	a.GET("/", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestOTelErrorBranch(t *testing.T) {
	a := flash.New()
	a.Use(OTel("svc"))
	a.GET("/u/:id", func(c flash.Ctx) error { return errors.New("boom") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/u/1", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from default error handler, got %d", rec.Code)
	}
}

func TestOTelWithConfig_Options(t *testing.T) {
	a := flash.New()
	a.Use(OTelWithConfig(OTelConfig{
		ServiceName:    "svc",
		RecordDuration: true,
		Filter: func(c flash.Ctx) bool {
			return c.Path() == "/healthz" // skip tracing but proceed
		},
		Status: func(code int, err error) (codes.Code, string) {
			if code >= 400 && code < 500 {
				return codes.Error, "client error"
			}
			if err != nil || code >= 500 {
				return codes.Error, http.StatusText(code)
			}
			return codes.Ok, ""
		},
	}))

	a.GET("/", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	a.GET("/healthz", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	a.GET("/bad", func(c flash.Ctx) error { return c.String(http.StatusBadRequest, "bad") })

	for path, want := range map[string]int{"/": http.StatusOK, "/healthz": http.StatusOK, "/bad": http.StatusBadRequest} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		a.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: got %d want %d", path, rec.Code, want)
		}
	}
}

func TestOTelWithConfig_CustomizationsBranches(t *testing.T) {
	// Use no-op tracer and a no-op propagator to exercise non-nil paths
	noopTracer := trace.NewNoopTracerProvider().Tracer("test")
	noopProp := propagation.NewCompositeTextMapPropagator()

	a := flash.New()
	a.Use(OTelWithConfig(OTelConfig{
		Tracer:      noopTracer,
		Propagator:  noopProp,
		ServiceName: "svc2",
		SpanName: func(c flash.Ctx) string {
			// Return empty to ensure default branch fallback
			return ""
		},
		Attributes: func(c flash.Ctx) []attribute.KeyValue {
			return []attribute.KeyValue{attribute.String("custom.attr", "v")}
		},
		ExtraAttributes: []attribute.KeyValue{attribute.String("extra.attr", "x")},
		Status: func(code int, err error) (codes.Code, string) {
			// Explicitly mark OK with custom description
			return codes.Ok, ""
		},
	}))

	a.GET("/x", func(c flash.Ctx) error {
		// set route name to ensure http.route attribute path covered
		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(context.Background())
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestOTelWithConfig_SpanNameOverride_And_NoWrite(t *testing.T) {
	a := flash.New()
	a.Use(OTelWithConfig(OTelConfig{
		ServiceName: "svc3",
		SpanName:    func(c flash.Ctx) string { return "CUSTOM NAME" }, // non-empty override branch
		// default Status mapping used; ensure default branch is exercised
	}))

	// Handler writes nothing and returns nil -> status remains 0 inside middleware, should default to 200
	a.GET("/empty", func(c flash.Ctx) error { return nil })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected default 200 when no write, got %d", rec.Code)
	}
}
