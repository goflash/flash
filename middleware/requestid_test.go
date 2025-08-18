package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v2"
)

func TestRequestIDSetsHeaderAndContext(t *testing.T) {
	a := flash.New()
	a.Use(RequestID())
	a.GET("/", func(c flash.Ctx) error {
		if _, ok := RequestIDFromContext(c.Context()); !ok {
			t.Fatalf("request id missing")
		}
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("header missing")
	}
}

func TestRequestIDCustomHeader(t *testing.T) {
	a := flash.New()
	a.Use(RequestID(RequestIDConfig{Header: "X-CID"}))
	a.GET("/", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("X-CID") == "" {
		t.Fatalf("custom header missing")
	}
}

func TestRequestIDFromContextMissing(t *testing.T) {
	a := flash.New()
	a.GET("/", func(c flash.Ctx) error {
		if _, ok := RequestIDFromContext(c.Context()); ok {
			t.Fatalf("expected no request id")
		}
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
}

func TestRequestIDFromContextTypeMismatch(t *testing.T) {
	ctx := context.WithValue(context.Background(), ridKey{}, 123)
	if _, ok := RequestIDFromContext(ctx); ok {
		t.Fatalf("expected false on wrong type")
	}
}
