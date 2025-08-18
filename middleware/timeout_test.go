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
	a.GET("/slow", func(c *flash.Ctx) error {
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
	a.GET("/slow2", func(c *flash.Ctx) error {
		time.Sleep(20 * time.Millisecond)
		return c.String(http.StatusOK, "ok")
	}, Timeout(TimeoutConfig{Duration: 5 * time.Millisecond, OnTimeout: func(c *flash.Ctx) { called = true }, ErrorResponse: func(c *flash.Ctx) error { return c.String(599, "custom") }}))

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
	a.GET("/fast", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") }, Timeout(TimeoutConfig{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("expected 200 ok, got %d %q", rec.Code, rec.Body.String())
	}
}
