package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash"
)

func TestCSRFProtection(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())

	// Handlers
	a.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "get") })
	a.POST("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "post") })

	// GET should set cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("csrf cookie not set")
	}
	ck := rec.Result().Cookies()[0]

	// POST without header should be forbidden
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(ck)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	// POST with matching header should pass
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(ck)
	req.Header.Set("X-CSRF-Token", ck.Value)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCSRFSafeMethodsSetCookieOnly(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	a.HEAD("/h", func(c *flash.Ctx) error { return c.String(http.StatusOK, "") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/h", nil)
	a.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("cookie not set on safe method")
	}
}

func TestCSRFInvalidHeader(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	// Register both GET and POST for same path to ensure middleware runs on GET to set cookie
	path := "/p"
	a.GET(path, func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	a.POST(path, func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	// obtain cookie via GET
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	a.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected csrf cookie to be set")
	}
	ck := cookies[0]
	// mismatched header
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(ck)
	req.Header.Set("X-CSRF-Token", "bad")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403")
	}
}

func TestCSRFEnsureCookieNotOverwriteExisting(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	// First request sets cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	a.ServeHTTP(rec, req)
	cks := rec.Result().Cookies()
	if len(cks) == 0 {
		t.Fatalf("no cookie")
	}
	first := cks[0]
	// Second GET should not change cookie value; middleware may not resend cookie
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(first)
	a.ServeHTTP(rec2, req2)
	cks2 := rec2.Result().Cookies()
	if len(cks2) > 0 && cks2[0].Value != first.Value {
		t.Fatalf("csrf cookie should be preserved")
	}
}

func TestCSRFPostNoCookieForbidden(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	a.POST("/x", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when no csrf cookie")
	}
}

func TestCSRFOptionsSetsCookie(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	// Register an OPTIONS handler so next() runs
	a.OPTIONS("/opt", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/opt", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("expected CSRF cookie set on OPTIONS")
	}
}

func TestCSRFPostWithEmptyCookieForbidden(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	a.POST("/p2", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/p2", nil)
	// Add empty CSRF cookie to trigger missing-token branch
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: ""})
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when csrf cookie empty, got %d", rec.Code)
	}
}

func TestCSRFPostHeaderWrongLengthForbidden(t *testing.T) {
	a := flash.New()
	a.Use(CSRF())
	a.POST("/z", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	// Obtain a valid cookie via GET
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/z", nil)
	// Need a GET handler to call next()
	a.GET("/z", func(c *flash.Ctx) error { return c.String(http.StatusOK, "g") })
	a.ServeHTTP(rec, req)
	cks := rec.Result().Cookies()
	if len(cks) == 0 {
		t.Fatalf("no csrf cookie")
	}
	ck := cks[0]
	// POST with header of different length to force subtleConstantTimeCompare len mismatch
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/z", nil)
	req.AddCookie(ck)
	req.Header.Set("X-CSRF-Token", ck.Value+"x")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on wrong-length header")
	}
}

func TestCSRFCustomConfig(t *testing.T) {
	a := flash.New()
	cfg := CSRFConfig{
		CookieName:     "TKN",
		HeaderName:     "X-My-CSRF",
		TokenLength:    8,
		CookiePath:     "/c",
		CookieDomain:   "example.com",
		CookieSecure:   false,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		TTL:            time.Hour,
	}
	a.Use(CSRF(cfg))

	path := "/c"
	a.GET(path, func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })
	a.POST(path, func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// GET sets custom cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	a.ServeHTTP(rec, req)
	cks := rec.Result().Cookies()
	if len(cks) == 0 || cks[0].Name != "TKN" {
		t.Fatalf("expected custom csrf cookie 'TKN', got %#v", cks)
	}
	ck := cks[0]
	if ck.Path != "/c" || ck.Domain != "example.com" || ck.HttpOnly != true || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie attributes not honored: %#v", ck)
	}
	// POST with correct custom header should pass
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(ck)
	req.Header.Set("X-My-CSRF", ck.Value)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid custom header, got %d", rec.Code)
	}
	// POST with missing custom header should be forbidden
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(ck)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when custom header missing, got %d", rec.Code)
	}
}
