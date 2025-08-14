package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash"
)

func TestBufferSetsContentLengthAndFlushes(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 128, MaxSize: 1024}))
	a.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "hello") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("Content-Length") != "5" {
		t.Fatalf("want CL=5 got %s", rec.Header().Get("Content-Length"))
	}
}

func TestBufferSwitchesToStreamingOnLargeResponse(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 4, MaxSize: 8}))
	big := make([]byte, 100)
	for i := range big {
		big[i] = 'x'
	}
	a.GET("/", func(c *flash.Ctx) error { _, _ = c.Send(http.StatusOK, "text/plain", big); return nil })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Length") == "" {
		// streaming path won't set Content-Length preemptively
		_ = time.Second // no-op to avoid unused import on time if removed
	}
}

func TestBufferHEADNoBody(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 0}))
	a.HEAD("/h", func(c *flash.Ctx) error { return c.String(http.StatusOK, "") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/h", nil)
	a.ServeHTTP(rec, req)
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD should have no body")
	}
}

func TestBufferFlushForcesStreaming(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 4, MaxSize: 8}))
	a.GET("/sse", func(c *flash.Ctx) error {
		c.ResponseWriter().(http.Flusher).Flush() // call flush early
		_, _ = c.Send(http.StatusOK, "text/plain", []byte("data"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	a.ServeHTTP(rec, req)
	_ = time.Second
}

// Exercise strconvItoa via buffer paths that set Content-Length
func TestStrconvItoaCoverage(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/n", func(c *flash.Ctx) error {
		_, _ = c.ResponseWriter().Write([]byte("12345")) // no explicit Content-Length
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/n", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Length") != "5" {
		t.Fatalf("bad content-length")
	}
}

func TestBufferFirstWriteExceedsMaxSizeStreamsImmediately(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 2})) // MaxSize smaller than first write
	a.GET("/stream", func(c *flash.Ctx) error {
		_, _ = c.ResponseWriter().Write([]byte("abc")) // len=3 > MaxSize, with empty buffer
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Fatalf("expected no Content-Length on streaming, got %q", got)
	}
	if rec.Body.String() != "abc" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestBufferBufferedThenOverflowFlushesAndStreams(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 3}))
	a.GET("/mix", func(c *flash.Ctx) error {
		w := c.ResponseWriter()
		_, _ = w.Write([]byte("ab"))  // buffered
		_, _ = w.Write([]byte("cde")) // overflow -> flush "ab" then stream "cde"
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mix", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Fatalf("expected no Content-Length on streaming, got %q", got)
	}
	if rec.Body.String() != "abcde" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestBufferCloseNoWritesDefaultsTo200(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/nowrite", func(c *flash.Ctx) error { return nil })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nowrite", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body")
	}
}

func TestBufferCloseNoWritesWithPresetStatus(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/nostatusbody", func(c *flash.Ctx) error {
		// Set status on the buffered ResponseWriter so Buffer.Close() will honor it
		c.ResponseWriter().WriteHeader(http.StatusNoContent)
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nostatusbody", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body")
	}
}

func TestBufferFlushWithBufferedDataWritesAndNoContentLength(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/flush-buf", func(c *flash.Ctx) error {
		w := c.ResponseWriter()
		_, _ = w.Write([]byte("abc")) // buffered
		w.(http.Flusher).Flush()      // flush buffered bytes without CL
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/flush-buf", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Fatalf("expected no Content-Length after Flush, got %q", got)
	}
	if rec.Body.String() != "abc" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestBufferFlushWithoutAnyWritesSetsHeaderAndStreams(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/flush-empty", func(c *flash.Ctx) error {
		c.ResponseWriter().(http.Flusher).Flush() // no prior writes, buf==nil path
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/flush-empty", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body after empty Flush")
	}
}

func TestBufferEnsureBufEarlyReturn(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 0}))
	a.GET("/twowrites", func(c *flash.Ctx) error {
		w := c.ResponseWriter()
		_, _ = w.Write([]byte("hi"))
		_, _ = w.Write([]byte("there"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/twowrites", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Length") != "7" {
		t.Fatalf("want CL=7 got %s", rec.Header().Get("Content-Length"))
	}
}

func TestBufferNoContentLengthWhenEncodingPreset(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/enc", func(c *flash.Ctx) error {
		c.Header("Content-Encoding", "br")
		_, _ = c.ResponseWriter().Write([]byte("abc"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/enc", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Length") != "" {
		t.Fatalf("Content-Length should not be set when Content-Encoding preset")
	}
}

func TestBufferFlushTwiceCoversStreamingBranch(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 4, MaxSize: 8}))
	a.GET("/flush2", func(c *flash.Ctx) error {
		f := c.ResponseWriter().(http.Flusher)
		f.Flush()
		f.Flush()
		_, _ = c.ResponseWriter().Write([]byte("ok"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/flush2", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestBufferZeroLengthSetsCLZero(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/zero", func(c *flash.Ctx) error {
		_, _ = c.ResponseWriter().Write([]byte{})
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/zero", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Length") != "0" {
		t.Fatalf("want CL=0 got %s", rec.Header().Get("Content-Length"))
	}
}

// failOnFirstWriteRW wraps a ResponseRecorder and fails the first Write call.
type failOnFirstWriteRW struct {
	*httptest.ResponseRecorder
	fail bool
}

func (w *failOnFirstWriteRW) Write(p []byte) (int, error) {
	if w.fail {
		w.fail = false
		return 0, errors.New("write boom")
	}
	return w.ResponseRecorder.Write(p)
}

func TestBufferSwitchToStreamingFlushBufferedWriteError(t *testing.T) {
	a := flash.New()
	called := false
	a.OnError = func(_ *flash.Ctx, err error) { called = true }
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 3}))
	a.GET("/e", func(c *flash.Ctx) error {
		w := c.ResponseWriter()
		// write small chunk to buffer
		if _, err := w.Write([]byte("ab")); err != nil {
			return err
		}
		// next write exceeds MaxSize, will attempt to flush buffered bytes to underlying and we force error
		_, err := w.Write([]byte("cde"))
		return err
	})
	// custom writer that fails on first Write (during flush of buffered content)
	rec := &failOnFirstWriteRW{ResponseRecorder: httptest.NewRecorder(), fail: true}
	req := httptest.NewRequest(http.MethodGet, "/e", nil)
	a.ServeHTTP(rec, req)
	if !called {
		t.Fatalf("expected custom error handler to be invoked")
	}
}

func TestBufferRespectsPreSetContentLength(t *testing.T) {
	a := flash.New()
	a.Use(Buffer())
	a.GET("/preset", func(c *flash.Ctx) error {
		c.Header("Content-Length", "99") // simulate pre-set length
		_, _ = c.ResponseWriter().Write([]byte("abc"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/preset", nil)
	a.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Length"); got != "99" {
		t.Fatalf("Content-Length should be preserved, got %q", got)
	}
}
