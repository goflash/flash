package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/goflash/flash/v1"
)

// TimeoutConfig configures the timeout middleware.
// Duration sets the timeout. OnTimeout is called when a timeout occurs. ErrorResponse can customize the timeout response.
type TimeoutConfig struct {
	Duration      time.Duration          // request timeout duration
	OnTimeout     func(*flash.Ctx)       // optional callback on timeout
	ErrorResponse func(*flash.Ctx) error // optional custom error response
}

// timeoutWriter buffers header mutations locally and writes to the real writer under a mutex.
// After a timeout occurs, all handler writes are dropped, while the timeout path writes exclusively.
type timeoutWriter struct {
	w           http.ResponseWriter
	mu          sync.Mutex
	timedOut    bool
	header      http.Header
	wroteHeader bool
	status      int
}

func newTimeoutWriter(w http.ResponseWriter) *timeoutWriter {
	h := make(http.Header, len(w.Header()))
	for k, v := range w.Header() {
		vv := make([]string, len(v))
		copy(vv, v)
		h[k] = vv
	}
	return &timeoutWriter{w: w, header: h}
}

func (tw *timeoutWriter) Header() http.Header { return tw.header }

// copy headers from src to dst (replace semantics)
func copyHeaders(dst, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for k, v := range src {
		vals := make([]string, len(v))
		copy(vals, v)
		dst[k] = vals
	}
}

// writeHeaderLocked copies handler headers and writes the status if not already written. Caller must hold tw.mu.
func (tw *timeoutWriter) writeHeaderLocked(status int) {
	if tw.wroteHeader {
		return
	}
	copyHeaders(tw.w.Header(), tw.header)
	tw.w.WriteHeader(status)
	tw.wroteHeader = true
	tw.status = status
}

func (tw *timeoutWriter) WriteHeader(status int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return
	}
	tw.writeHeaderLocked(status)
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return len(b), nil
	}
	if !tw.wroteHeader {
		tw.writeHeaderLocked(http.StatusOK)
	}
	return tw.w.Write(b)
}

// Optional interfaces passthroughs
func (tw *timeoutWriter) Flush() {
	if f, ok := tw.w.(http.Flusher); ok {
		tw.mu.Lock()
		defer tw.mu.Unlock()
		if tw.timedOut {
			return
		}
		f.Flush()
	}
}

// timeoutResponder has its own header map to be used by the timeout path only.
// It serializes writes to the underlying writer using the timeoutWriter mutex.
type timeoutResponder struct {
	tw          *timeoutWriter
	header      http.Header
	wroteHeader bool
}

func newTimeoutResponder(tw *timeoutWriter) *timeoutResponder {
	return &timeoutResponder{tw: tw, header: make(http.Header)}
}

func (tr *timeoutResponder) Header() http.Header { return tr.header }

func (tr *timeoutResponder) writeHeaderLocked(status int) {
	if tr.tw.wroteHeader {
		return
	}
	// Mark timed out to drop any future handler writes
	tr.tw.timedOut = true
	copyHeaders(tr.tw.w.Header(), tr.header)
	tr.tw.w.WriteHeader(status)
	tr.tw.wroteHeader = true
	tr.tw.status = status
	tr.wroteHeader = true
}

func (tr *timeoutResponder) WriteHeader(status int) {
	tr.tw.mu.Lock()
	defer tr.tw.mu.Unlock()
	tr.writeHeaderLocked(status)
}

func (tr *timeoutResponder) Write(b []byte) (int, error) {
	tr.tw.mu.Lock()
	defer tr.tw.mu.Unlock()
	// Ensure header is written
	if !tr.tw.wroteHeader {
		tr.writeHeaderLocked(http.StatusOK)
	}
	// Mark timed out (again) and write body
	tr.tw.timedOut = true
	return tr.tw.w.Write(b)
}

// Timeout returns middleware that applies a timeout to the request context.
// If the handler does not complete within Duration, a 504 Gateway Timeout is returned.
func Timeout(cfg TimeoutConfig) flash.Middleware {
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			ctx, cancel := context.WithTimeout(c.Context(), cfg.Duration)
			defer cancel()

			// Update the original request context for any downstream usage in timeout path
			c.SetRequest(c.Request().WithContext(ctx))

			// Prepare a shallow copy of the context for the handler goroutine to avoid races
			copyCtx := *c
			tw := newTimeoutWriter(c.ResponseWriter())
			copyCtx.SetResponseWriter(tw)
			copyCtx.SetRequest(c.Request())

			done := make(chan error, 1)
			go func() { done <- next(&copyCtx) }()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				// If handler completed concurrently, prefer it to avoid double writes
				select {
				case err := <-done:
					return err
				default:
				}
				// Route timeout response through timeoutResponder to serialize writes
				tr := newTimeoutResponder(tw)
				c.SetResponseWriter(tr)
				if cfg.OnTimeout != nil {
					cfg.OnTimeout(c)
				}
				if cfg.ErrorResponse != nil {
					return cfg.ErrorResponse(c)
				}
				// Default 504 response without sharing handler headers
				body := http.StatusText(http.StatusGatewayTimeout)
				tr.Header().Set("Content-Type", "text/plain; charset=utf-8")
				tr.Header().Set("Content-Length", strconv.Itoa(len(body)))
				tr.WriteHeader(http.StatusGatewayTimeout)
				_, _ = tr.Write([]byte(body))
				return nil
			}
		}
	}
}
