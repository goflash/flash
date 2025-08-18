package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v1"
)

type captureHandler struct{ rec []slog.Record }

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.rec = append(h.rec, r)
	return nil
}
func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(name string) slog.Handler       { return h }

func TestLoggerMiddlewareEmitsLog(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger())
	a.GET("/x", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
}

func TestLoggerDefaultStatusAndRequestIDAttr(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	// Ensure RequestID runs within the request so request_id is available when Logger logs
	a.Use(Logger(), RequestID())
	// Handler that does not write any response headers/body
	a.GET("/y", func(c *flash.Ctx) error { return nil })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/y", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	// Use the last record in case other tests also logged
	recIdx := len(h.rec) - 1
	var status int
	var hasRID bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "status" {
			// Value.Any returns the Go value; handle common numeric types
			if v, ok := a.Value.Any().(int64); ok {
				status = int(v)
			} else if v, ok := a.Value.Any().(int); ok {
				status = v
			}
		}
		if a.Key == "request_id" {
			hasRID = true
		}
		return true
	})
	if status != 200 {
		t.Fatalf("expected default status 200, got %d", status)
	}
	if !hasRID {
		t.Fatalf("expected request_id attribute")
	}
}

func TestLoggerStatusDefaultWhenNoWrite(t *testing.T) {
	a := flash.New()
	a.Use(Logger())
	a.GET("/noop", func(c *flash.Ctx) error { return nil })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/noop", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { // default 200 from recorder
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
