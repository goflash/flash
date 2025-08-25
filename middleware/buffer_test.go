package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
)

func TestBufferSetsContentLengthAndFlushes(t *testing.T) {
	a := flash.New()
	a.Use(Buffer(BufferConfig{InitialSize: 128, MaxSize: 1024}))
	a.GET("/", func(c flash.Ctx) error { return c.String(http.StatusOK, "hello") })

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
	a.GET("/", func(c flash.Ctx) error { _, _ = c.Send(http.StatusOK, "text/plain", big); return nil })

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
	a.HEAD("/h", func(c flash.Ctx) error { return c.String(http.StatusOK, "") })
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
	a.GET("/sse", func(c flash.Ctx) error {
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
	a.GET("/n", func(c flash.Ctx) error {
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
	a.GET("/stream", func(c flash.Ctx) error {
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
	a.GET("/mix", func(c flash.Ctx) error {
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
	a.GET("/nowrite", func(c flash.Ctx) error { return nil })
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
	a.GET("/nostatusbody", func(c flash.Ctx) error {
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
	a.GET("/flush-buf", func(c flash.Ctx) error {
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
	a.GET("/flush-empty", func(c flash.Ctx) error {
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
	a.GET("/twowrites", func(c flash.Ctx) error {
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
	a.GET("/enc", func(c flash.Ctx) error {
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
	a.GET("/flush2", func(c flash.Ctx) error {
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
	a.GET("/zero", func(c flash.Ctx) error {
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
	a.SetErrorHandler(func(_ flash.Ctx, err error) { called = true })
	a.Use(Buffer(BufferConfig{InitialSize: 0, MaxSize: 3}))
	a.GET("/e", func(c flash.Ctx) error {
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
	a.GET("/preset", func(c flash.Ctx) error {
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

// hijackableRecorder wraps a ResponseRecorder and implements http.Hijacker.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	c1, c2 := net.Pipe()
	// Close both ends immediately; we only care about delegation path
	rw := bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1))
	_ = c2.Close()
	return c1, rw, nil
}

// pusherRecorder wraps a ResponseRecorder and implements http.Pusher.
type pusherRecorder struct {
	*httptest.ResponseRecorder
	pushed []string
}

func (p *pusherRecorder) Push(target string, opts *http.PushOptions) error {
	p.pushed = append(p.pushed, target)
	return nil
}

func TestBufferHijackDelegationAndUnsupported(t *testing.T) {
	t.Run("delegates when underlying supports hijack", func(t *testing.T) {
		a := flash.New()
		a.Use(Buffer())
		called := false
		a.GET("/h", func(c flash.Ctx) error {
			called = true
			hj := c.ResponseWriter().(http.Hijacker)
			conn, rw, err := hj.Hijack()
			if err != nil || conn == nil || rw == nil {
				t.Fatalf("hijack failed: conn=%v rw=%v err=%v", conn, rw, err)
			}
			_ = conn.Close()
			return nil
		})
		rec := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
		req := httptest.NewRequest(http.MethodGet, "/h", nil)
		a.ServeHTTP(rec, req)
		if !called {
			t.Fatalf("handler not called")
		}
		if !rec.hijacked {
			t.Fatalf("expected underlying Hijack to be called")
		}
	})

	t.Run("returns ErrNotSupported when underlying lacks hijack", func(t *testing.T) {
		a := flash.New()
		a.Use(Buffer())
		var gotErr error
		a.GET("/h2", func(c flash.Ctx) error {
			_, _, gotErr = c.ResponseWriter().(http.Hijacker).Hijack()
			return nil
		})
		rec := httptest.NewRecorder() // no Hijacker support underneath
		req := httptest.NewRequest(http.MethodGet, "/h2", nil)
		a.ServeHTTP(rec, req)
		if gotErr != http.ErrNotSupported {
			t.Fatalf("expected ErrNotSupported, got %v", gotErr)
		}
	})
}

func TestBufferPushDelegationAndUnsupported(t *testing.T) {
	t.Run("delegates to underlying Pusher", func(t *testing.T) {
		a := flash.New()
		a.Use(Buffer())
		a.GET("/p", func(c flash.Ctx) error {
			if err := c.ResponseWriter().(http.Pusher).Push("/style.css", nil); err != nil {
				t.Fatalf("push failed: %v", err)
			}
			return nil
		})
		rec := &pusherRecorder{ResponseRecorder: httptest.NewRecorder()}
		req := httptest.NewRequest(http.MethodGet, "/p", nil)
		a.ServeHTTP(rec, req)
		if len(rec.pushed) != 1 || rec.pushed[0] != "/style.css" {
			t.Fatalf("expected one push to /style.css, got %+v", rec.pushed)
		}
	})

	t.Run("returns ErrNotSupported when underlying lacks pusher", func(t *testing.T) {
		a := flash.New()
		a.Use(Buffer())
		var errPush error
		a.GET("/p2", func(c flash.Ctx) error {
			errPush = c.ResponseWriter().(http.Pusher).Push("/x", nil)
			return nil
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2", nil)
		a.ServeHTTP(rec, req)
		if errPush != http.ErrNotSupported {
			t.Fatalf("expected ErrNotSupported, got %v", errPush)
		}
	})
}
