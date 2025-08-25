package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goflash/flash/v2"
)

func TestCORSPreflightAndHeaders(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"*"}, Methods: []string{"GET", "POST"}, Headers: []string{"X-A"}, Expose: []string{"X-E"}, MaxAge: 600}))

	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	// Register OPTIONS so middleware runs for preflight
	a.OPTIONS("/x", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	// Preflight
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Access-Control-Request-Method", "GET")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight=%d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("missing allow methods")
	}
	if rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("missing allow headers")
	}

	// Actual
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Expose-Headers") == "" {
		t.Fatalf("missing expose headers")
	}
}

func TestCORSDefaultMethodsPreflight(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"*"}})) // Methods empty => default
	a.OPTIONS("/x", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Access-Control-Request-Method", "GET")
	a.ServeHTTP(rec, req)
	am := rec.Header().Get("Access-Control-Allow-Methods")
	if am == "" || !strings.Contains(am, "GET") || !strings.Contains(am, "POST") || !strings.Contains(am, "HEAD") {
		t.Fatalf("default allow methods missing, got %q", am)
	}
}

func TestCORSUniqMethods(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"*"}, Methods: []string{"GET", "GET", "POST"}}))
	a.OPTIONS("/y", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/y", nil)
	req.Header.Set("Access-Control-Request-Method", "GET")
	a.ServeHTTP(rec, req)
	am := rec.Header().Get("Access-Control-Allow-Methods")
	if strings.Count(am, "GET") != 1 {
		t.Fatalf("methods not unique: %q", am)
	}
}

func TestCORSOptionsWithoutPreflightHeader(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"*"}}))
	a.OPTIONS("/noop", func(c flash.Ctx) error { return c.String(http.StatusOK, "") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/noop", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on non-preflight OPTIONS, got %d", rec.Code)
	}
}

func TestCORSCredentialsHeader(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"https://example.com"}, Credentials: true}))
	a.GET("/cred", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cred", nil)
	req.Header.Set("Origin", "https://example.com")
	a.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("expected Access-Control-Allow-Credentials=true")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("expected Access-Control-Allow-Origin to be https://example.com, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSWildcardWithCredentialsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic when using wildcard origin with credentials")
		}
	}()

	// This should panic due to security check
	CORS(CORSConfig{Origins: []string{"*"}, Credentials: true})
}
