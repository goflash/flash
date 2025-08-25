package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v2"
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
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
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
	a.GET("/y", func(c flash.Ctx) error { return nil })
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
	a.GET("/noop", func(c flash.Ctx) error { return nil })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/noop", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { // default 200 from recorder
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLoggerWithExcludeFields(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithExcludeFields("user_agent", "remote")))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "test-agent")
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var hasUserAgent, hasRemote bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "user_agent" {
			hasUserAgent = true
		}
		if a.Key == "remote" {
			hasRemote = true
		}
		return true
	})
	if hasUserAgent {
		t.Fatalf("user_agent should be excluded")
	}
	if hasRemote {
		t.Fatalf("remote should be excluded")
	}
}

func TestLoggerWithCustomAttributesFunc(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithCustomAttributes(func(c flash.Ctx) []any {
		return []any{"custom_key", "custom_value", "user_id", "123"}
	})))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var hasCustomKey, hasUserID bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "custom_key" {
			hasCustomKey = true
		}
		if a.Key == "user_id" {
			hasUserID = true
		}
		return true
	})
	if !hasCustomKey {
		t.Fatalf("custom_key should be present")
	}
	if !hasUserID {
		t.Fatalf("user_id should be present")
	}
}

func TestLoggerWithCustomAttributesFromContext(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger())
	a.GET("/test", func(c flash.Ctx) error {
		// Add custom attributes to context
		attrs := NewLoggerAttributes("context_key", "context_value", "operation", "test")
		ctx := WithLoggerAttributes(c.Context(), attrs)
		c.SetRequest(c.Request().WithContext(ctx))
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var hasContextKey, hasOperation bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "context_key" {
			hasContextKey = true
		}
		if a.Key == "operation" {
			hasOperation = true
		}
		return true
	})
	if !hasContextKey {
		t.Fatalf("context_key should be present")
	}
	if !hasOperation {
		t.Fatalf("operation should be present")
	}
}

func TestLoggerWithCustomMessage(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithMessage("custom_request")))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	if h.rec[recIdx].Message != "custom_request" {
		t.Fatalf("expected message 'custom_request', got '%s'", h.rec[recIdx].Message)
	}
}

func TestLoggerWithMultipleOptions(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(
		WithExcludeFields("user_agent"),
		WithCustomAttributes(func(c flash.Ctx) []any {
			return []any{"test_key", "test_value"}
		}),
		WithMessage("multi_test"),
	))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "test-agent")
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var hasUserAgent, hasTestKey bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "user_agent" {
			hasUserAgent = true
		}
		if a.Key == "test_key" {
			hasTestKey = true
		}
		return true
	})
	if hasUserAgent {
		t.Fatalf("user_agent should be excluded")
	}
	if !hasTestKey {
		t.Fatalf("test_key should be present")
	}
	if h.rec[recIdx].Message != "multi_test" {
		t.Fatalf("expected message 'multi_test', got '%s'", h.rec[recIdx].Message)
	}
}

func TestLoggerAttributesFromContextReturnsNil(t *testing.T) {
	ctx := context.Background()
	attrs := LoggerAttributesFromContext(ctx)
	if attrs != nil {
		t.Fatalf("expected nil attributes for empty context")
	}
}

func TestLoggerAttributesFromContextWithInvalidValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), LoggerAttributeKey{}, "invalid")
	attrs := LoggerAttributesFromContext(ctx)
	if attrs != nil {
		t.Fatalf("expected nil attributes for invalid value")
	}
}

func TestLoggerAttributesFromContextWithValidValue(t *testing.T) {
	originalAttrs := NewLoggerAttributes("key", "value")
	ctx := WithLoggerAttributes(context.Background(), originalAttrs)
	attrs := LoggerAttributesFromContext(ctx)
	if attrs == nil {
		t.Fatalf("expected valid attributes")
	}
	if len(attrs.attrs) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(attrs.attrs))
	}
	if attrs.attrs[0] != "key" || attrs.attrs[1] != "value" {
		t.Fatalf("expected key/value pair, got %v", attrs.attrs)
	}
}

func TestNewLoggerAttributes(t *testing.T) {
	attrs := NewLoggerAttributes("key1", "value1", "key2", "value2")
	if len(attrs.attrs) != 4 {
		t.Fatalf("expected 4 attributes, got %d", len(attrs.attrs))
	}
	if attrs.attrs[0] != "key1" || attrs.attrs[1] != "value1" {
		t.Fatalf("expected first key/value pair")
	}
	if attrs.attrs[2] != "key2" || attrs.attrs[3] != "value2" {
		t.Fatalf("expected second key/value pair")
	}
}

func TestLoggerAttributesAdd(t *testing.T) {
	attrs := NewLoggerAttributes("key1", "value1")
	attrs.Add("key2", "value2", "key3", "value3")
	if len(attrs.attrs) != 6 {
		t.Fatalf("expected 6 attributes, got %d", len(attrs.attrs))
	}
	if attrs.attrs[2] != "key2" || attrs.attrs[3] != "value2" {
		t.Fatalf("expected added key/value pairs")
	}
}

func TestLoggerWithEmptyCustomAttributesFunc(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithCustomAttributes(func(c flash.Ctx) []any {
		return nil
	})))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	// Should still log standard fields
	recIdx := len(h.rec) - 1
	var hasMethod bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "method" {
			hasMethod = true
		}
		return true
	})
	if !hasMethod {
		t.Fatalf("standard fields should still be logged")
	}
}

func TestLoggerWithRequestIDExcluded(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithExcludeFields("request_id")), RequestID())
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var hasRequestID bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "request_id" {
			hasRequestID = true
		}
		return true
	})
	if hasRequestID {
		t.Fatalf("request_id should be excluded")
	}
}

func TestLoggerWithAllStandardFieldsExcluded(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithExcludeFields("method", "path", "route", "status", "duration_ms", "remote", "user_agent", "request_id")))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	recIdx := len(h.rec) - 1
	var attrCount int
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		attrCount++
		return true
	})
	if attrCount != 0 {
		t.Fatalf("expected no attributes when all standard fields are excluded, got %d", attrCount)
	}
}

func TestLoggerWithCustomAttributesFuncReturningEmptySlice(t *testing.T) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithCustomAttributes(func(c flash.Ctx) []any {
		return []any{}
	})))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)
	if len(h.rec) == 0 {
		t.Fatalf("no logs captured")
	}
	// Should still log standard fields
	recIdx := len(h.rec) - 1
	var hasMethod bool
	h.rec[recIdx].Attrs(func(a slog.Attr) bool {
		if a.Key == "method" {
			hasMethod = true
		}
		return true
	})
	if !hasMethod {
		t.Fatalf("standard fields should still be logged")
	}
}

func BenchmarkLogger(b *testing.B) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger())
	a.GET("/bench", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		a.ServeHTTP(rec, req)
	}
}

func BenchmarkLoggerWithCustomAttributes(b *testing.B) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithCustomAttributes(func(c flash.Ctx) []any {
		return []any{"user_id", "123", "operation", "benchmark"}
	})))
	a.GET("/bench", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		a.ServeHTTP(rec, req)
	}
}

func BenchmarkLoggerWithExcludeFields(b *testing.B) {
	a := flash.New()
	h := &captureHandler{}
	a.SetLogger(slog.New(h))
	a.Use(Logger(WithExcludeFields("user_agent", "remote")))
	a.GET("/bench", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		a.ServeHTTP(rec, req)
	}
}
