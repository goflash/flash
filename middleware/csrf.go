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
//
// This middleware implements the double-submit cookie pattern for CSRF protection.
// A cryptographically secure token is generated and stored in both a cookie and
// expected in a header for unsafe HTTP methods (POST, PUT, PATCH, DELETE).
//
// Security considerations:
//   - Use HTTPS in production (CookieSecure: true)
//   - Set appropriate SameSite policy (SameSiteLaxMode recommended)
//   - Use HttpOnly cookies to prevent XSS token theft
//   - Ensure TokenLength is sufficient (32 bytes minimum recommended)
//   - Set reasonable TTL to balance security and user experience
//
// Example:
//
//	cfg := middleware.CSRFConfig{
//		CookieName:     "_csrf",
//		HeaderName:     "X-CSRF-Token",
//		TokenLength:    32,
//		CookieSecure:   true,
//		CookieHTTPOnly: true,
//		CookieSameSite: http.SameSiteLaxMode,
//		TTL:            12 * time.Hour,
//	}
//	app.Use(middleware.CSRF(cfg))
type CSRFConfig struct {
	// CookieName specifies the name of the CSRF cookie.
	// Common values: "_csrf", "csrf_token", "XSRF-TOKEN".
	CookieName string
	// HeaderName specifies the name of the header where the CSRF token is expected.
	// Common values: "X-CSRF-Token", "X-XSRF-Token", "X-CSRF-Header".
	HeaderName string
	// TokenLength sets the length of the generated token in bytes.
	// Recommended: 32 bytes (256 bits) for adequate security.
	// The actual token string will be longer due to base64 encoding.
	TokenLength int
	// CookiePath sets the path attribute of the CSRF cookie.
	// Use "/" to apply to the entire domain.
	CookiePath string
	// CookieDomain sets the domain attribute of the CSRF cookie.
	// Leave empty for current domain only.
	CookieDomain string
	// CookieSecure sets the Secure flag on the CSRF cookie.
	// Should be true in production (HTTPS required).
	CookieSecure bool
	// CookieHTTPOnly sets the HttpOnly flag on the CSRF cookie.
	// Should be true to prevent XSS attacks from stealing the token.
	CookieHTTPOnly bool
	// CookieSameSite sets the SameSite policy for the CSRF cookie.
	// Recommended: http.SameSiteLaxMode for most applications.
	CookieSameSite http.SameSite
	// TTL sets the expiration time for the CSRF cookie.
	// Balance security (shorter) with user experience (longer).
	// Common values: 12 hours, 24 hours, 7 days.
	TTL time.Duration
}

// DefaultCSRFConfig returns a safe default configuration for CSRF protection.
//
// The default configuration provides strong security with reasonable usability:
//   - 32-byte tokens (256 bits of entropy)
//   - Secure, HttpOnly cookies
//   - SameSite=Lax policy
//   - 12-hour expiration
//   - Standard cookie and header names
//
// Example:
//
//	app.Use(middleware.CSRF()) // uses DefaultCSRFConfig()
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
//
// Behavior:
//   - For safe methods (GET, HEAD, OPTIONS): sets CSRF cookie if missing, then continues
//   - For unsafe methods (POST, PUT, PATCH, DELETE): validates token in both cookie and header
//   - Returns 403 Forbidden if token is missing or invalid
//   - Uses constant-time comparison to prevent timing attacks
//
// Performance notes:
//   - Token generation uses crypto/rand for cryptographic security
//   - Constant-time comparison prevents timing-based attacks
//   - Cookie validation only occurs for unsafe methods
//   - Minimal overhead for safe methods (just cookie setting)
//
// Security features:
//   - Double-submit pattern prevents CSRF attacks
//   - Cryptographically secure random tokens
//   - Constant-time token comparison
//   - Configurable cookie security attributes
//
// Example (using defaults):
//
//	app.Use(middleware.CSRF())
//
// Example (custom configuration):
//
//	app.Use(middleware.CSRF(middleware.CSRFConfig{
//		CookieName:     "csrf_token",
//		HeaderName:     "X-CSRF-Header",
//		TokenLength:    64, // stronger tokens
//		CookieSecure:   true,
//		CookieHTTPOnly: true,
//		CookieSameSite: http.SameSiteStrictMode,
//		TTL:            24 * time.Hour,
//	}))
//
// Client-side usage:
//
//	// JavaScript: read token from cookie and send in header
//	const token = document.cookie.match('_csrf=([^;]+)')[1];
//	fetch('/api/data', {
//		method: 'POST',
//		headers: { 'X-CSRF-Token': token },
//		body: JSON.stringify(data)
//	});
func CSRF(cfgs ...CSRFConfig) flash.Middleware {
	cfg := DefaultCSRFConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
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

// ensureCSRFCookie sets a CSRF cookie if one doesn't already exist.
// Called for safe methods to ensure the token is available for subsequent unsafe requests.
func ensureCSRFCookie(c flash.Ctx, cfg CSRFConfig) {
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

// generateCSRFToken creates a cryptographically secure random token.
// Uses crypto/rand for security and base64.RawURLEncoding for URL-safe output.
//
// Example:
//
//	token := generateCSRFToken(32) // 32 bytes = 256 bits of entropy
func generateCSRFToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// compareTokens compares two tokens using constant-time comparison.
// This prevents timing attacks that could reveal token information.
func compareTokens(a, b string) bool {
	return subtleConstantTimeCompare(a, b)
}

// subtleConstantTimeCompare compares two strings in constant time.
// This prevents timing attacks by ensuring the comparison always takes
// the same amount of time regardless of where the strings differ.
//
// Security: This is a simplified constant-time comparison. For production
// use, consider using crypto/subtle.ConstantTimeCompare for maximum security.
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
