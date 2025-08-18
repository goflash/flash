package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/goflash/flash/v2"
)

// GzipConfig configures the gzip middleware.
// Level sets the gzip compression level (see compress/gzip). Defaults to gzip.DefaultCompression.
type GzipConfig struct {
	Level int // gzip compression level
}

// gzipPools is a global map of sync.Pool keyed by compression level.
// This avoids repeated allocations of gzip.Writer, which are expensive to create and GC.
// Using sync.Pool here is safe because each request gets a fresh writer, and the pool is keyed by level.
var gzipPools sync.Map // map[int]*sync.Pool

// getGzipWriter returns a pooled gzip.Writer for the given level and output writer.
// This minimizes allocations and GC pressure in the hot path.
func getGzipWriter(level int, w io.Writer) (*gzip.Writer, func()) {
	poolAny, _ := gzipPools.LoadOrStore(level, &sync.Pool{New: func() any {
		// gzip.NewWriterLevel is expensive; pool for each level
		gw, _ := gzip.NewWriterLevel(io.Discard, level)
		return gw
	}})
	pool := poolAny.(*sync.Pool)
	gw := pool.Get().(*gzip.Writer)
	gw.Reset(w)
	put := func() {
		// Always close and reset before putting back to pool
		_ = gw.Close()
		gw.Reset(io.Discard)
		pool.Put(gw)
	}
	return gw, put
}

// Gzip returns middleware that enables gzip compression when the client sends Accept-Encoding: gzip.
// The compression level can be configured via GzipConfig. HEAD requests are never compressed.
func Gzip(cfgs ...GzipConfig) flash.Middleware {
	cfg := GzipConfig{Level: gzip.DefaultCompression}
	if len(cfgs) > 0 {
		c := cfgs[0]
		if c.Level != 0 {
			cfg.Level = c.Level
		}
	}
	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			r := c.Request()
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || c.Method() == http.MethodHead {
				return next(c)
			}

			grw := &gzipResponseWriter{rw: c.ResponseWriter(), level: cfg.Level}
			c.SetResponseWriter(grw)
			defer grw.Close()

			return next(c)
		}
	}
}

type gzipResponseWriter struct {
	rw          http.ResponseWriter
	gz          *gzip.Writer
	put         func()
	level       int
	wroteHeader bool
	useGzip     bool // set on first write/header
}

func (g *gzipResponseWriter) Header() http.Header { return g.rw.Header() }

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true

	// decide whether to gzip
	enc := g.Header().Get("Content-Encoding")
	if enc != "" && enc != "identity" {
		g.useGzip = false
		g.rw.WriteHeader(status)
		return
	}
	if status == http.StatusNoContent || status == http.StatusNotModified {
		g.useGzip = false
		g.rw.WriteHeader(status)
		return
	}

	g.useGzip = true
	g.Header().Del("Content-Length")
	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Add("Vary", "Accept-Encoding")
	g.rw.WriteHeader(status)
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	if !g.useGzip {
		return g.rw.Write(p)
	}
	if g.gz == nil {
		gw, put := getGzipWriter(g.level, g.rw)
		g.gz, g.put = gw, put
	}
	return g.gz.Write(p)
}

func (g *gzipResponseWriter) Close() error {
	if g.gz != nil {
		if g.put != nil {
			g.put()
			g.gz, g.put = nil, nil
			return nil
		}
		return g.gz.Close()
	}
	return nil
}

// Support http.Flusher if underlying supports it.
func (g *gzipResponseWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.rw.(http.Flusher); ok {
		f.Flush()
	}
}

// Ensure interfaces are implemented
var _ http.ResponseWriter = (*gzipResponseWriter)(nil)
var _ http.Flusher = (*gzipResponseWriter)(nil)
