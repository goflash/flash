package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUseNoArgsNoop(t *testing.T) {
	a := New().(*DefaultApp)
	before := len(a.middleware)
	a.Use()
	if len(a.middleware) != before {
		t.Fatalf("expected no change")
	}
}

func TestAppGETAndMiddlewareAndErrorHandler(t *testing.T) {
	a := New()
	called := 0
	a.Use(func(next Handler) Handler { return func(c Ctx) error { called++; return next(c) } })

	a.GET("/ping", func(c Ctx) error { return c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if called == 0 {
		t.Fatalf("global middleware not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Body.String(); got != "pong" {
		t.Fatalf("body=%q", got)
	}
}

func TestAppNotFoundAndMethodNA(t *testing.T) {
	a := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ping", nil)

	a.GET("/ping", func(c Ctx) error { return c.String(http.StatusOK, "pong") })

	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleHTTPAndMountAndStatic(t *testing.T) {
	a := New()
	// HandleHTTP
	a.HandleHTTP(http.MethodGet, "/std", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "std") }))
	// Mount
	a.Mount("/m", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "m") }))
	// Static: use httptest's FileServer like behavior by writing a simple handler under prefix
	// Here we simulate by registering a GET that matches the internal pattern
	a.GET("/files/*filepath", func(c Ctx) error { return c.String(http.StatusOK, "file") })

	tests := []struct{ path, want string }{
		{"/std", "std"},
		{"/m", "m"},
		{"/files/x.txt", "file"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != tt.want {
			t.Fatalf("path=%s code=%d body=%q", tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestANYRegistersAllMethods(t *testing.T) {
	a := New()
	a.ANY("/ping", func(c Ctx) error { return c.String(http.StatusOK, c.Method()) })
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead}
	for _, m := range methods {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(m, "/ping", nil)
		a.ServeHTTP(rec, req)
		if rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("method %s not allowed", m)
		}
	}
}

func TestCustomNotFoundAndMethodNAAndOnError(t *testing.T) {
	a := New()
	a.SetNotFoundHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404); io.WriteString(w, "NF") }))
	a.SetMethodNotAllowedHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(405); io.WriteString(w, "MNA") }))

	// NotFound
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	a.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "NF" {
		t.Fatalf("notfound body=%q", got)
	}

	// MethodNA
	a.GET("/x", func(c Ctx) error { return c.String(http.StatusOK, "ok") })
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	a.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "MNA" {
		t.Fatalf("methodna body=%q", got)
	}

	// OnError default 500
	a = New()
	a.GET("/e", func(c Ctx) error { return io.ErrUnexpectedEOF })
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/e", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatalf("default onerror code=%d", rec.Code)
	}

	// Custom OnError
	a = New()
	a.SetErrorHandler(func(c Ctx, err error) { _ = c.String(418, "teapot") })
	a.GET("/e", func(c Ctx) error { return io.ErrUnexpectedEOF })
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/e", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != 418 || rec.Body.String() != "teapot" {
		t.Fatalf("custom onerror: %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandleCustomMethod(t *testing.T) {
	a := New()
	a.Handle("PURGE", "/c", func(c Ctx) error { return c.String(http.StatusOK, "purged") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PURGE", "/c", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "purged" {
		t.Fatalf("bad PURGE")
	}
}

func TestStaticServesFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.Static("/static", dir)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
		t.Fatalf("static failed: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSetLoggerAndLoggerFallback(t *testing.T) {
	a := New().(*DefaultApp)
	// SetLogger path
	l := slog.Default()
	a.SetLogger(l)
	if a.Logger() != l {
		t.Fatalf("SetLogger not used")
	}
	// nil fallback path
	a.logger = nil
	if a.Logger() == nil {
		t.Fatalf("Logger() should fallback to default")
	}
}

func TestUseNoopOnEmpty(t *testing.T) {
	a := New().(*DefaultApp)
	// should not panic and should not change middleware length
	before := len(a.middleware)
	a.Use()
	after := len(a.middleware)
	if before != after {
		t.Fatalf("Use() with no args should be no-op")
	}
}
