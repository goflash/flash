package middleware

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/goflash/flash/v1"
)

// BufferConfig configures the write-buffering middleware.
// InitialSize preallocates the response buffer. MaxSize limits buffering; if exceeded, switches to streaming.
type BufferConfig struct {
	InitialSize int // preallocated buffer size
	MaxSize     int // max buffer size before switching to streaming
}

// bufPool is a global sync.Pool for *bytes.Buffer used by the Buffer middleware.
// This reduces allocations and GC pressure for each request, especially for small/medium responses.
// Buffers are always Reset before reuse, and never shared between requests.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// Buffer returns middleware that wraps the ResponseWriter with a pooled buffer to reduce syscalls and set Content-Length.
// Not recommended for streaming/SSE. Apply before handlers that generate bounded payloads.
func Buffer(cfgs ...BufferConfig) flash.Middleware {
	cfg := BufferConfig{InitialSize: 0, MaxSize: 0}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
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

func (b *bufferedRW) Header() http.Header { return b.rw.Header() }

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

func (b *bufferedRW) WriteHeader(status int) { b.status = status }

func (b *bufferedRW) Write(p []byte) (int, error) {
	if b.streaming {
		b.writeHeaderIfNeeded(false)
		return b.rw.Write(p)
	}
	b.ensureBuf()
	// If exceeding MaxSize, switch to streaming
	if b.cfg.MaxSize > 0 && b.buf.Len()+len(p) > b.cfg.MaxSize {
		// flush buffered content without Content-Length
		b.writeHeaderIfNeeded(false)
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

// Close flushes the buffer and sets Content-Length for GET/HEAD if possible.
// This enables zero-allocation HEAD responses: if the handler does not write a body,
// no buffer is allocated, and only headers are sent. For GET, Content-Length is set
// unless Content-Encoding is present. This is a key optimization for API and static routes.
func (b *bufferedRW) Close() error {
	if b.streaming {
		b.release()
		return nil
	}
	if b.buf == nil {
		// nothing written; still honor header if set (HEAD/204/304)
		b.writeHeaderIfNeeded(false)
		return nil
	}
	// set Content-Length if not already set and no Content-Encoding present
	h := b.Header()
	if h.Get("Content-Length") == "" && h.Get("Content-Encoding") == "" {
		h.Set("Content-Length", strconvItoa(b.buf.Len()))
	}
	b.writeHeaderIfNeeded(true)
	if b.buf.Len() > 0 {
		_, _ = b.rw.Write(b.buf.Bytes())
	}
	b.release()
	return nil
}

func (b *bufferedRW) writeHeaderIfNeeded(withLength bool) {
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

func (b *bufferedRW) Flush() {
	// Flush forces streaming and forwards to underlying if supported
	if b.streaming {
		if f, ok := b.rw.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	// write out what we have without Content-Length
	b.writeHeaderIfNeeded(false)
	if b.buf != nil && b.buf.Len() > 0 {
		_, _ = b.rw.Write(b.buf.Bytes())
		b.release()
	}
	b.streaming = true
	if f, ok := b.rw.(http.Flusher); ok {
		f.Flush()
	}
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
