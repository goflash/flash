package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
)

func TestTimeoutMiddleware(t *testing.T) {
	a := flash.New()
	a.GET("/slow", func(c flash.Ctx) error {
		time.Sleep(50 * time.Millisecond)
		return c.String(http.StatusOK, "ok")
	}, Timeout(TimeoutConfig{Duration: 10 * time.Millisecond}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", rec.Code)
	}
}

func TestTimeoutOnTimeoutAndCustomErrorResponse(t *testing.T) {
	called := false
	a := flash.New()
	a.GET("/slow2", func(c flash.Ctx) error {
		time.Sleep(20 * time.Millisecond)
		return c.String(http.StatusOK, "ok")
	}, Timeout(TimeoutConfig{Duration: 5 * time.Millisecond, OnTimeout: func(c flash.Ctx) { called = true }, ErrorResponse: func(c flash.Ctx) error { return c.String(599, "custom") }}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow2", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != 599 || rec.Body.String() != "custom" {
		t.Fatalf("expected custom 599, got %d %q", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatalf("OnTimeout not called")
	}
}

func TestTimeoutDefaultDurationNoTimeout(t *testing.T) {
	a := flash.New()
	// Duration is zero -> defaults internally to 5s; handler returns immediately so no timeout
	a.GET("/fast", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") }, Timeout(TimeoutConfig{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("expected 200 ok, got %d %q", rec.Code, rec.Body.String())
	}
}

// simpleWriter is a minimal http.ResponseWriter that does NOT implement http.Flusher
type simpleWriter struct {
	header http.Header
	code   int
}

func (w *simpleWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *simpleWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *simpleWriter) WriteHeader(status int)      { w.code = status }

func TestTimeoutWriter_Flush_PassthroughAndNoops(t *testing.T) {
	// Passthrough when underlying supports http.Flusher and not timed out
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)
	if rec.Flushed {
		t.Fatalf("unexpected flushed before calling Flush")
	}
	tw.Flush()
	if !rec.Flushed {
		t.Fatalf("expected underlying recorder to be flushed")
	}

	// When timed out, Flush should be a no-op
	rec2 := httptest.NewRecorder()
	tw2 := newTimeoutWriter(rec2)
	tw2.mu.Lock()
	tw2.timedOut = true
	tw2.mu.Unlock()
	tw2.Flush()
	if rec2.Flushed {
		t.Fatalf("expected no flush after timeout")
	}

	// When underlying does not implement Flusher, Flush should be a no-op but not panic
	sw := &simpleWriter{}
	tw3 := newTimeoutWriter(sw)
	// Should not panic
	tw3.Flush()
}

func TestNewTimeoutWriter_CopiesHeaderAndIsolation(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("A", "v1")
	rec.Header().Add("A", "v2")

	tw := newTimeoutWriter(rec)

	// Copy-on-create semantics
	if got := tw.Header()["A"]; len(got) != 2 || got[0] != "v1" || got[1] != "v2" {
		t.Fatalf("expected copied header values, got %v", got)
	}
	// Mutations to original should not reflect into copied header
	rec.Header().Set("A", "mutated")
	if got := tw.Header().Get("A"); got != "v1" {
		t.Fatalf("expected original copy to remain, got %q", got)
	}
	// Mutations to copied header should not affect original
	tw.Header().Set("B", "x")
	if _, ok := rec.Header()["B"]; ok {
		t.Fatalf("did not expect original writer header to have B")
	}
}

func TestTimeoutWriter_Write_Behavior_DefaultAndAfterHeader(t *testing.T) {
	// Default path: Write triggers 200 and header copy
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)
	tw.Header().Set("X-Test", "yes")
	n, err := tw.Write([]byte("hi"))
	if err != nil || n != 2 {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on first write, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Test"); got != "yes" {
		t.Fatalf("expected header copied to underlying, got %q", got)
	}
	if rec.Body.String() != "hi" {
		t.Fatalf("expected body 'hi', got %q", rec.Body.String())
	}

	// Subsequent write should append and keep status
	_, _ = tw.Write([]byte("!"))
	if rec.Body.String() != "hi!" {
		t.Fatalf("expected body 'hi!', got %q", rec.Body.String())
	}

	// Respect pre-set status via WriteHeader
	rec2 := httptest.NewRecorder()
	tw2 := newTimeoutWriter(rec2)
	tw2.Header().Set("Y-Test", "1")
	tw2.WriteHeader(201)
	n, err = tw2.Write([]byte("foo"))
	if err != nil || n != 3 {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
	if rec2.Code != 201 {
		t.Fatalf("expected status 201, got %d", rec2.Code)
	}
	if got := rec2.Header().Get("Y-Test"); got != "1" {
		t.Fatalf("expected header copied to underlying, got %q", got)
	}
	// Timed out path: Write should be no-op besides returning len(b)
	tw2.mu.Lock()
	tw2.timedOut = true
	tw2.mu.Unlock()
	n, err = tw2.Write([]byte("bar"))
	if err != nil || n != 3 {
		t.Fatalf("timed out write failed: n=%d err=%v", n, err)
	}
	if rec2.Body.String() != "foo" {
		t.Fatalf("expected body unchanged after timeout, got %q", rec2.Body.String())
	}
}

func TestTimeoutResponder_Write_CoversBothBranches(t *testing.T) {
	// Branch 1: tw.wroteHeader == false, Write should set 200 and headers
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)
	tr := newTimeoutResponder(tw)
	tr.Header().Set("K", "v")
	n, err := tr.Write([]byte("body"))
	if err != nil || n != 4 {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("K"); got != "v" {
		t.Fatalf("expected header set, got %q", got)
	}
	if rec.Body.String() != "body" {
		t.Fatalf("expected body 'body', got %q", rec.Body.String())
	}
	if !tw.timedOut {
		t.Fatalf("expected timedOut to be true after timeoutResponder write")
	}

	// Branch 2: tw.wroteHeader == true, Write should append body without changing status
	rec2 := httptest.NewRecorder()
	tw2 := newTimeoutWriter(rec2)
	tr2 := newTimeoutResponder(tw2)
	tr2.Header().Set("H", "x")
	tr2.WriteHeader(504)
	n, err = tr2.Write([]byte(" more"))
	if err != nil || n != 5 {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
	if rec2.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected status 504, got %d", rec2.Code)
	}
	if got := rec2.Header().Get("H"); got != "x" {
		t.Fatalf("expected header persisted, got %q", got)
	}
}

func TestTimeoutWriterWriteHeaderEdgeCases(t *testing.T) {
	// Test WriteHeader when already timed out
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)

	// Simulate timeout
	tw.mu.Lock()
	tw.timedOut = true
	tw.mu.Unlock()

	tw.WriteHeader(http.StatusCreated)

	// Should not write header when timed out
	if rec.Code != http.StatusOK {
		t.Errorf("expected no status change when timed out, got %d", rec.Code)
	}
}

func TestTimeoutWriterWriteHeaderAlreadyWritten(t *testing.T) {
	// Test WriteHeader when already written
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)

	// Write header first time
	tw.WriteHeader(http.StatusCreated)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}

	// Write header second time - should be ignored
	tw.WriteHeader(http.StatusBadRequest)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status to remain 201, got %d", rec.Code)
	}
}

func TestCopyHeadersWithExistingHeaders(t *testing.T) {
	// Test copyHeaders function with existing headers in destination
	src := make(http.Header)
	src.Set("X-Source", "source-value")
	src.Add("X-Multi", "value1")
	src.Add("X-Multi", "value2")

	dst := make(http.Header)
	dst.Set("X-Existing", "existing-value")
	dst.Set("X-Override", "old-value")

	copyHeaders(dst, src)

	// Existing headers should be removed
	if dst.Get("X-Existing") != "" {
		t.Error("expected existing headers to be removed")
	}
	if dst.Get("X-Override") != "" {
		t.Error("expected overridden headers to be removed")
	}

	// Source headers should be copied
	if dst.Get("X-Source") != "source-value" {
		t.Errorf("expected X-Source to be copied, got %s", dst.Get("X-Source"))
	}

	// Multi-value headers should be copied
	values := dst["X-Multi"]
	if len(values) != 2 || values[0] != "value1" || values[1] != "value2" {
		t.Errorf("expected multi-value header to be copied, got %v", values)
	}
}

func TestTimeoutResponderWriteHeaderAlreadyWritten(t *testing.T) {
	// Test timeoutResponder WriteHeader when underlying already wrote header
	rec := httptest.NewRecorder()
	tw := newTimeoutWriter(rec)

	// Write header on underlying writer first
	tw.WriteHeader(http.StatusOK)

	tr := newTimeoutResponder(tw)
	tr.Header().Set("X-Test", "value")
	tr.WriteHeader(http.StatusGatewayTimeout)

	// Should not change status when already written
	if rec.Code != http.StatusOK {
		t.Errorf("expected status to remain 200, got %d", rec.Code)
	}

	// Should NOT mark as timed out because header was already written
	// This hits the early return path in writeHeaderLocked
	tw.mu.Lock()
	timedOut := tw.timedOut
	tw.mu.Unlock()
	if timedOut {
		t.Error("expected timeoutWriter NOT to be marked as timed out when header already written")
	}
}

func TestTimeoutMiddlewareWithPanicInHandler(t *testing.T) {
	// Test timeout middleware when handler panics
	a := flash.New()
	a.Use(Timeout(TimeoutConfig{Duration: 100 * time.Millisecond}))

	a.GET("/panic", func(c flash.Ctx) error {
		panic("test panic")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)

	// Should handle panic gracefully
	if rec.Code == 0 {
		t.Error("expected some status code to be set")
	}
}

func TestTimeoutMiddlewareWithCustomErrorResponse(t *testing.T) {
	// Test timeout middleware with custom error response
	a := flash.New()
	customCalled := false

	a.Use(Timeout(TimeoutConfig{
		Duration: 1 * time.Millisecond,
		ErrorResponse: func(c flash.Ctx) error {
			customCalled = true
			return c.String(http.StatusRequestTimeout, "Custom timeout")
		},
	}))

	a.GET("/slow", func(c flash.Ctx) error {
		time.Sleep(10 * time.Millisecond) // Longer than timeout
		return c.String(http.StatusOK, "success")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	a.ServeHTTP(rec, req)

	if !customCalled {
		t.Error("expected custom error response to be called")
	}
	if rec.Code != http.StatusRequestTimeout {
		t.Errorf("expected status 408, got %d", rec.Code)
	}
	if rec.Body.String() != "Custom timeout" {
		t.Errorf("expected custom timeout message, got %s", rec.Body.String())
	}
}
