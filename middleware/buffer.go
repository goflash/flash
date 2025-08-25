package middleware

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"sync"

	"github.com/goflash/flash/v2"
)

// BufferConfig configures the write-buffering middleware.
//
// InitialSize preallocates the internal response buffer capacity to reduce
// growth reallocations for small/medium payloads. MaxSize limits how many
// bytes are buffered before switching to streaming mode. When the buffered
// bytes would exceed MaxSize, the middleware flushes any buffered content
// without a Content-Length and routes subsequent writes directly to the
// underlying ResponseWriter.
//
// Notes and recommendations:
//   - Set MaxSize to a sensible ceiling to avoid unbounded memory use for very
//     large responses (MaxSize=0 means unbounded buffering).
//   - This middleware is not suitable for server-sent events or long-lived
//     streaming responses. Use it for bounded payloads (JSON, HTML, small files).
//   - For HEAD responses where no body is written, no buffer is allocated at all.
//
// Example:
//
//	app.Use(middleware.Buffer(middleware.BufferConfig{
//		InitialSize: 8 << 10, // 8KB
//		MaxSize:     1 << 20, // 1MB
//	}))
type BufferConfig struct {
	InitialSize int // preallocated buffer size
	MaxSize     int // max buffer size before switching to streaming
}

// bufPool is a global sync.Pool for *bytes.Buffer used by the Buffer middleware.
// This reduces allocations and GC pressure for each request, especially for small/medium responses.
// Buffers are always Reset before reuse, and never shared between requests.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// Buffer returns middleware that wraps the ResponseWriter with a pooled buffer
// to reduce syscalls and to set an accurate Content-Length when possible.
// Not recommended for streaming/SSE. Apply before handlers that generate
// bounded payloads.
//
// Behavior:
//   - Buffers writes in-memory up to MaxSize; beyond that, switches to streaming
//   - Sets Content-Length on close when safe (no Content-Encoding)
//   - Supports Flush passthrough and zero-allocation HEAD responses
//
// Example:
//
//	// Global buffering with defaults (unbounded). Prefer setting MaxSize.
//	app.Use(middleware.Buffer())
//
//	// Per-route configuration
//	app.GET("/report", handler, middleware.Buffer(middleware.BufferConfig{MaxSize: 2<<20}))
func Buffer(cfgs ...BufferConfig) flash.Middleware {
	cfg := BufferConfig{InitialSize: 0, MaxSize: 0}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			brw := &bufferedRW{rw: c.ResponseWriter(), cfg: cfg}
			c.SetResponseWriter(brw)
			defer brw.Close()
			return next(c)
		}
	}
}

type bufferedRW struct {
	rw          http.ResponseWriter
	cfg         BufferConfig
	buf         *bytes.Buffer
	status      int
	headWritten bool // whether we've written header to underlying
	streaming   bool // switched to passthrough
}

// Header returns the underlying response headers map.
// Example: set a header before the first write.
//
//	brw.Header().Set("Content-Type", "application/json")
func (b *bufferedRW) Header() http.Header { return b.rw.Header() }

// ensureBuf lazily acquires a buffer from the pool and applies InitialSize.
// Called on first buffered write.
func (b *bufferedRW) ensureBuf() {
	if b.buf != nil {
		return
	}
	// Get a buffer from the pool (or allocate if pool is empty)
	bb := bufPool.Get().(*bytes.Buffer)
	bb.Reset() // always reset before use
	if b.cfg.InitialSize > 0 {
		bb.Grow(b.cfg.InitialSize)
	}
	b.buf = bb
}

// WriteHeader records the status code. The header is written lazily on the
// first body write or during Close/Flush. If not set, defaults to 200 OK.
func (b *bufferedRW) WriteHeader(status int) { b.status = status }

// Write buffers the payload unless streaming mode has been enabled.
// If MaxSize would be exceeded by this write, buffered content is flushed and
// subsequent writes are streamed directly to the underlying writer.
//
// Example (switching to streaming): if MaxSize is 1MB and the handler writes
// 600KB then 600KB, the second write triggers a flush and streaming.
func (b *bufferedRW) Write(p []byte) (int, error) {
	if b.streaming {
		b.writeHeaderIfNeeded()
		return b.rw.Write(p)
	}
	b.ensureBuf()
	// If exceeding MaxSize, switch to streaming
	if b.cfg.MaxSize > 0 && b.buf.Len()+len(p) > b.cfg.MaxSize {
		// flush buffered content without Content-Length
		b.writeHeaderIfNeeded()
		if b.buf.Len() > 0 {
			if _, err := b.rw.Write(b.buf.Bytes()); err != nil {
				return 0, err
			}
			b.release()
		}
		b.streaming = true
		return b.rw.Write(p)
	}
	return b.buf.Write(p)
}

// Close flushes the buffer and sets Content-Length when possible.
//
// This enables zero-allocation HEAD responses: if the handler does not write a
// body, no buffer is allocated and only headers are sent. For GET, Content-Length
// is set unless Content-Encoding is present. This is a key optimization for API
// and static routes.
func (b *bufferedRW) Close() error {
	if b.streaming {
		b.release()
		return nil
	}
	if b.buf == nil {
		// nothing written; still honor header if set (HEAD/204/304)
		b.writeHeaderIfNeeded()
		return nil
	}
	// set Content-Length if not already set and no Content-Encoding present
	h := b.Header()
	if h.Get("Content-Length") == "" && h.Get("Content-Encoding") == "" {
		h.Set("Content-Length", strconvItoa(b.buf.Len()))
	}
	b.writeHeaderIfNeeded()
	if b.buf.Len() > 0 {
		_, _ = b.rw.Write(b.buf.Bytes())
	}
	b.release()
	return nil
}

// writeHeaderIfNeeded writes the header once, defaulting status to 200.
func (b *bufferedRW) writeHeaderIfNeeded() {
	if b.headWritten {
		return
	}
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	b.rw.WriteHeader(status)
	b.headWritten = true
}

// Flush forces streaming mode, flushes any buffered bytes to the underlying
// writer without a Content-Length, and forwards Flush if supported.
// Suitable for long-polling style responses that start buffered then stream.
func (b *bufferedRW) Flush() {
	// Flush forces streaming and forwards to underlying if supported
	if b.streaming {
		if f, ok := b.rw.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	// write out what we have without Content-Length
	b.writeHeaderIfNeeded()
	if b.buf != nil && b.buf.Len() > 0 {
		_, _ = b.rw.Write(b.buf.Bytes())
		b.release()
	}
	b.streaming = true
	if f, ok := b.rw.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates to the underlying ResponseWriter if it implements
// http.Hijacker. This is necessary for WebSocket upgrades or raw TCP access.
// If the underlying writer does not support hijacking, an error is returned.
func (b *bufferedRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := b.rw.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push delegates HTTP/2 server push to the underlying ResponseWriter if it
// implements http.Pusher. If not supported, Push returns http.ErrNotSupported.
func (b *bufferedRW) Push(target string, opts *http.PushOptions) error {
	if p, ok := b.rw.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (b *bufferedRW) release() {
	if b.buf != nil {
		// Always reset before putting back to pool to avoid data leaks
		b.buf.Reset()
		bufPool.Put(b.buf)
		b.buf = nil
	}
}

// compile-time assertions
var _ http.ResponseWriter = (*bufferedRW)(nil)
var _ http.Flusher = (*bufferedRW)(nil)
var _ http.Hijacker = (*bufferedRW)(nil)
var _ http.Pusher = (*bufferedRW)(nil)

// minimal itoa to avoid fmt in hot path
func strconvItoa(i int) string {
	if i == 0 {
		return "0"
	}
	// max len for int64 is 20, int is smaller; allocate small stack buf
	var a [20]byte
	pos := len(a)
	n := i
	for n > 0 {
		pos--
		a[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(a[pos:])
}
