package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goflash/flash"
)

func TestGzipMiddlewareCompressesWhenAccepted(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, strings.Repeat("x", 100)) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("no gzip header")
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	_, _ = io.ReadAll(zr)
	_ = zr.Close()
}

func TestGzipNotAppliedOnHEAD(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.HEAD("/x", func(c *flash.Ctx) error { return c.String(http.StatusOK, "") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatalf("gzip should not be set for HEAD")
	}
}

func TestGzipNotAppliedWhenEncodingPreset(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/x", func(c *flash.Ctx) error {
		c.Header("Content-Encoding", "br")
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatalf("should not gzip when encoding preset")
	}
}

func TestGzipNotAppliedOnNoContentOrNotModified(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/n", func(c *flash.Ctx) error { c.ResponseWriter().WriteHeader(http.StatusNoContent); return nil })
	a.GET("/m", func(c *flash.Ctx) error { c.ResponseWriter().WriteHeader(http.StatusNotModified); return nil })
	for _, p := range []string{"/n", "/m"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		a.ServeHTTP(rec, req)
		if rec.Header().Get("Content-Encoding") == "gzip" {
			t.Fatalf("should not gzip %s", p)
		}
	}
}

func TestGzipFlushBranch(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/f", func(c *flash.Ctx) error {
		// Write some data first so gzip writer is initialized
		_, _ = c.ResponseWriter().Write([]byte("hello"))
		if f, ok := c.ResponseWriter().(http.Flusher); ok {
			f.Flush()
		}
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/f", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestGzipCloseWhenNoWriter(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/nowriter", func(c *flash.Ctx) error {
		// don't write anything, Close should no-op
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nowriter", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestGzipCloseWithoutPutCallsClose(t *testing.T) {
	// Manually construct gzipResponseWriter to hit branch where put is nil but gz not nil
	rec := httptest.NewRecorder()
	g := &gzipResponseWriter{rw: rec, level: gzip.DefaultCompression}
	g.WriteHeader(http.StatusOK) // sets useGzip and header
	// Manually create a gzip.Writer and assign without setting put
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	g.gz = zw
	if err := g.Close(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestGzipNotAppliedWithoutAcceptEncoding(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/plain", func(c *flash.Ctx) error { return c.String(http.StatusOK, "hello") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plain", nil)
	// no Accept-Encoding header
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatalf("should not gzip without Accept-Encoding")
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestGzipWithCustomLevelCompresses(t *testing.T) {
	a := flash.New()
	a.Use(Gzip(GzipConfig{Level: gzip.BestSpeed}))
	a.GET("/lvl", func(c *flash.Ctx) error { return c.String(http.StatusOK, "xxxxxxxxxxxxxxxxxxxx") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lvl", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip encoding")
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader err: %v", err)
	}
	_, _ = io.ReadAll(zr)
	_ = zr.Close()
}

func TestGzipAppliedWhenContentEncodingIdentity(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/id", func(c *flash.Ctx) error {
		c.Header("Content-Encoding", "identity")
		return c.String(http.StatusOK, "hello world")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/id", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip despite identity preset, got %q", rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader err: %v", err)
	}
	_, _ = io.ReadAll(zr)
	_ = zr.Close()
}

func TestGzipWriteHeaderCalledTwiceUsesFirst(t *testing.T) {
	a := flash.New()
	a.Use(Gzip())
	a.GET("/tw", func(c *flash.Ctx) error {
		w := c.ResponseWriter()
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusAccepted) // should be ignored
		_, _ = w.Write([]byte("data"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tw", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 from first WriteHeader, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip encoding")
	}
}
