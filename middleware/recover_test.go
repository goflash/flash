package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
)

func TestRecoverMiddleware(t *testing.T) {
	a := flash.New()
	a.Use(Recover())
	a.GET("/panic", func(c flash.Ctx) error { panic("boom") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRecoverMiddlewareWithCustomErrorResponse(t *testing.T) {
	a := flash.New()
	customErrorCalled := false
	a.Use(Recover(RecoverConfig{
		ErrorResponse: func(c flash.Ctx, err interface{}) error {
			customErrorCalled = true
			return c.String(http.StatusBadRequest, "Custom error response")
		},
	}))
	a.GET("/panic", func(c flash.Ctx) error { panic("test panic") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)

	if !customErrorCalled {
		t.Error("custom error response was not called")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec.Body.String() != "Custom error response" {
		t.Fatalf("expected 'Custom error response', got %q", rec.Body.String())
	}
}

func TestRecoverMiddlewareWithOnPanic(t *testing.T) {
	a := flash.New()
	panicCalled := false
	var panicValue interface{}

	a.Use(Recover(RecoverConfig{
		OnPanic: func(c flash.Ctx, err interface{}) {
			panicCalled = true
			panicValue = err
		},
	}))
	a.GET("/panic", func(c flash.Ctx) error { panic("test panic value") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)

	// Give the goroutine time to execute
	time.Sleep(10 * time.Millisecond)

	if !panicCalled {
		t.Error("OnPanic callback was not called")
	}
	if panicValue != "test panic value" {
		t.Errorf("expected panic value 'test panic value', got %v", panicValue)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRecoverMiddlewareWithPanicInCallback(t *testing.T) {
	a := flash.New()
	a.Use(Recover(RecoverConfig{
		OnPanic: func(c flash.Ctx, err interface{}) {
			// This callback itself panics, but should be protected
			panic("callback panic")
		},
	}))
	a.GET("/panic", func(c flash.Ctx) error { panic("original panic") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)

	// Give the goroutine time to execute
	time.Sleep(10 * time.Millisecond)

	// Should still return 500 despite callback panic
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRecoverMiddlewareNoPanic(t *testing.T) {
	a := flash.New()
	callbackCalled := false

	a.Use(Recover(RecoverConfig{
		OnPanic: func(c flash.Ctx, err interface{}) {
			callbackCalled = true
		},
	}))
	a.GET("/normal", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "normal response")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/normal", nil)
	a.ServeHTTP(rec, req)

	if callbackCalled {
		t.Error("OnPanic callback should not be called for normal requests")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "normal response" {
		t.Fatalf("expected 'normal response', got %q", rec.Body.String())
	}
}
