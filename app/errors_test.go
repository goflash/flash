package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultErrorHandlerNoDoubleWrite(t *testing.T) {
	a := New()
	a.OnError = defaultErrorHandler
	a.GET("/w", func(c *Ctx) error {
		_ = c.String(http.StatusTeapot, "x") // already wrote
		return io.ErrUnexpectedEOF
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", rec.Code)
	}
}

func TestMethodNotAllowedHandler(t *testing.T) {
	h := methodNotAllowedHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
