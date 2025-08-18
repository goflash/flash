// Package middleware provides optional CSRF protection middleware for flash.
// This middleware uses a double-submit cookie pattern and is suitable for APIs and web apps.
// Usage: app.Use(mw.CSRF(mw.CSRFConfig{...}))
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
)

// CSRFConfig configures the CSRF middleware.
// CookieName and HeaderName control where the token is stored and checked.
// TokenLength sets the length of the generated token (in bytes).
// Cookie* fields control cookie properties. TTL sets cookie expiration.
type CSRFConfig struct {
	CookieName     string        // Name of the CSRF cookie
	HeaderName     string        // Name of the header to check for the token
	TokenLength    int           // Length of the generated token (bytes)
	CookiePath     string        // Path for the CSRF cookie
	CookieDomain   string        // Domain for the CSRF cookie
	CookieSecure   bool          // Secure flag for the CSRF cookie
	CookieHTTPOnly bool          // HttpOnly flag for the CSRF cookie
	CookieSameSite http.SameSite // SameSite policy
	TTL            time.Duration // Cookie expiration
}

// DefaultCSRFConfig returns a safe default config for CSRF protection.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookieName:     "_csrf",
		HeaderName:     "X-CSRF-Token",
		TokenLength:    32,
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteLaxMode,
		TTL:            12 * time.Hour,
	}
}

// CSRF returns middleware that provides CSRF protection using the double-submit cookie pattern.
// For unsafe methods, the token is checked in both cookie and header. For safe methods, a token is set if missing.
func CSRF(cfgs ...CSRFConfig) flash.Middleware {
	cfg := DefaultCSRFConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			// Only protect unsafe methods
			if c.Method() == http.MethodGet || c.Method() == http.MethodHead || c.Method() == http.MethodOptions {
				ensureCSRFCookie(c, cfg)
				return next(c)
			}
			cookie, err := c.Request().Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				return c.Status(http.StatusForbidden).String(http.StatusForbidden, "CSRF token missing")
			}
			headertok := c.Request().Header.Get(cfg.HeaderName)
			if headertok == "" || !compareTokens(cookie.Value, headertok) {
				return c.Status(http.StatusForbidden).String(http.StatusForbidden, "CSRF token invalid")
			}
			return next(c)
		}
	}
}

func ensureCSRFCookie(c *flash.Ctx, cfg CSRFConfig) {
	cookie, err := c.Request().Cookie(cfg.CookieName)
	if err == nil && cookie.Value != "" {
		return
	}
	tok := generateCSRFToken(cfg.TokenLength)
	http.SetCookie(c.ResponseWriter(), &http.Cookie{
		Name:     cfg.CookieName,
		Value:    tok,
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		Secure:   cfg.CookieSecure,
		HttpOnly: cfg.CookieHTTPOnly,
		SameSite: cfg.CookieSameSite,
		Expires:  time.Now().Add(cfg.TTL),
	})
}

func generateCSRFToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func compareTokens(a, b string) bool {
	return subtleConstantTimeCompare(a, b)
}

// subtleConstantTimeCompare compares two strings in constant time.
func subtleConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var res byte
	for i := 0; i < len(a); i++ {
		res |= a[i] ^ b[i]
	}
	return res == 0
}
