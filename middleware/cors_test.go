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

func TestCORSWithSpecificOrigins(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"https://allowed.com", "https://another.com"},
	}))

	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test allowed origin
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://allowed.com")
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://allowed.com" {
		t.Errorf("expected origin to be allowed, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	// Test disallowed origin
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected origin to be rejected, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSWithCredentials(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins:     []string{"https://trusted.com"},
		Credentials: true,
	}))

	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://trusted.com")
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected credentials to be allowed")
	}
}

func TestCORSPreflightWithRequestHeaders(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"*"},
		Headers: []string{"X-Custom-Header", "Authorization"},
	}))

	a.OPTIONS("/test", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header,Authorization")
	a.ServeHTTP(rec, req)

	allowedHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowedHeaders, "X-Custom-Header") {
		t.Error("expected X-Custom-Header to be allowed")
	}
	if !strings.Contains(allowedHeaders, "Authorization") {
		t.Error("expected Authorization to be allowed")
	}
}

func TestCORSNonPreflightRequest(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"https://example.com"},
		Expose:  []string{"X-Total-Count"},
	}))

	a.POST("/test", func(c flash.Ctx) error {
		c.Header("X-Total-Count", "100")
		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Error("expected origin to be set for non-preflight request")
	}
	if rec.Header().Get("Access-Control-Expose-Headers") != "X-Total-Count" {
		t.Error("expected expose headers to be set")
	}
}

func TestCORSWithNullOrigin(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"https://example.com"},
	}))

	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test null origin (should be rejected)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "null")
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected null origin to be rejected")
	}

	// Test empty origin (should be rejected)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Origin header set
	a.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected empty origin to be rejected")
	}
}

func TestCORSPreflightWithoutRequestMethod(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"*"}}))

	a.OPTIONS("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// OPTIONS request without Access-Control-Request-Method header (not a preflight)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	a.ServeHTTP(rec, req)

	// Should pass through to handler, not treated as preflight
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestCORSSecurityHeaders(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{Origins: []string{"https://example.com"}}))

	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	a.ServeHTTP(rec, req)

	// Check security headers are set
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options header")
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options header")
	}
}

func TestCORSPreflightWithInvalidMethod(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"https://example.com"},
		Methods: []string{"GET", "POST"}, // Limited methods
	}))

	a.OPTIONS("/test", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	// Preflight request for disallowed method
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE") // Not in allowed methods
	a.ServeHTTP(rec, req)

	// Should return 403 for disallowed method
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCORSPreflightWithInvalidHeaders(t *testing.T) {
	a := flash.New()
	a.Use(CORS(CORSConfig{
		Origins: []string{"https://example.com"},
		Headers: []string{"Content-Type"}, // Limited headers
	}))

	a.OPTIONS("/test", func(c flash.Ctx) error { return c.String(http.StatusNoContent, "") })

	// Preflight request with disallowed headers
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header,Authorization")
	a.ServeHTTP(rec, req)

	// Should return 403 for disallowed headers
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
