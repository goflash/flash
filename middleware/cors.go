package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/goflash/flash/v2"
)

// CORSConfig holds configuration for the CORS middleware.
//
// Origins, Methods, and Headers control allowed cross-origin requests.
// Expose lists headers exposed to the browser. Credentials enables cookies.
// MaxAge sets preflight cache duration (seconds).
//
// Security considerations:
//   - Use specific origins rather than "*" when possible
//   - Only expose headers that are necessary for your application
//   - Be cautious with Credentials=true as it allows cookies in cross-origin requests
//   - Set appropriate MaxAge to balance security and performance
//
// Example:
//
//	cfg := middleware.CORSConfig{
//		Origins:     []string{"https://app.example.com", "https://admin.example.com"},
//		Methods:     []string{"GET", "POST", "PUT", "DELETE"},
//		Headers:     []string{"Content-Type", "Authorization"},
//		Expose:      []string{"X-Total-Count"},
//		Credentials: true,
//		MaxAge:      86400, // 24 hours
//	}
//	app.Use(middleware.CORS(cfg))
type CORSConfig struct {
	// Origins specifies allowed origins for cross-origin requests.
	// If empty, no Access-Control-Allow-Origin header is set.
	// Use "*" to allow all origins (not recommended for production).
	Origins []string
	// Methods specifies allowed HTTP methods for cross-origin requests.
	// If empty, defaults to common methods: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
	Methods []string
	// Headers specifies allowed request headers for cross-origin requests.
	// Common values include: Content-Type, Authorization, X-Requested-With.
	Headers []string
	// Expose specifies response headers that browsers can access via JavaScript.
	// Common values include: X-Total-Count, X-Page-Count, X-Rate-Limit-*.
	Expose []string
	// Credentials enables sending cookies and authorization headers in cross-origin requests.
	// When true, sets Access-Control-Allow-Credentials: true.
	// Note: Cannot be used with Origins: ["*"].
	Credentials bool
	// MaxAge sets the duration (in seconds) that browsers can cache preflight responses.
	// This reduces the number of OPTIONS requests for subsequent requests.
	// Common values: 86400 (24 hours), 3600 (1 hour), 0 (no cache).
	MaxAge int
}

// CORS returns middleware that sets CORS headers and handles preflight requests
// according to the provided config with enhanced security features.
//
// Security Features:
//   - Origin validation with wildcard handling
//   - Prevents credential exposure with wildcard origins
//   - Validates requested methods and headers against allowed lists
//   - Adds security headers to prevent content type sniffing
//   - Proper handling of null and invalid origins
//
// Behavior:
//   - Sets Access-Control-Allow-Origin, -Credentials, -Expose-Headers on all responses
//   - For OPTIONS requests with Access-Control-Request-Method header (preflight):
//   - Validates requested method against allowed methods
//   - Validates requested headers against allowed headers
//   - Sets Access-Control-Allow-Methods, -Headers, -Max-Age
//   - Returns 204 No Content
//   - For other OPTIONS requests: passes through to handler
//   - For non-OPTIONS requests: passes through to handler
//
// Performance notes:
//   - Headers are computed once at middleware creation, not per request
//   - Origin validation uses efficient string matching
//   - Preflight responses are cached by browsers according to MaxAge
//   - No allocations in the hot path for header string joining
//
// Example:
//
//	// Simple CORS for API
//	app.Use(middleware.CORS(middleware.CORSConfig{
//		Origins: []string{"https://app.example.com"},
//		Methods: []string{"GET", "POST", "PUT", "DELETE"},
//		Headers: []string{"Content-Type", "Authorization"},
//	}))
//
// Example (with credentials and caching):
//
//	// CORS for authenticated API
//	app.Use(middleware.CORS(middleware.CORSConfig{
//		Origins:     []string{"https://app.example.com", "https://admin.example.com"},
//		Methods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
//		Headers:     []string{"Content-Type", "Authorization", "X-Requested-With"},
//		Expose:      []string{"X-Total-Count", "X-Rate-Limit-Remaining"},
//		Credentials: true,
//		MaxAge:      3600, // 1 hour
//	}))
//
// Security Best Practices:
//
//	// Production-ready CORS configuration
//	app.Use(middleware.CORS(middleware.CORSConfig{
//		Origins:     []string{"https://app.example.com"}, // Specific origins, not "*"
//		Methods:     []string{"GET", "POST", "PUT", "DELETE"}, // Only needed methods
//		Headers:     []string{"Content-Type", "Authorization"}, // Only needed headers
//		Credentials: true, // Only if needed
//		MaxAge:      86400, // 24 hours - balance security and performance
//	}))
func CORS(cfg CORSConfig) flash.Middleware {
	allowedMethods := uniqOrDefault(cfg.Methods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"})
	allowedMethodsStr := strings.Join(allowedMethods, ", ")
	allowedHeaders := cfg.Headers
	allowedHeadersStr := strings.Join(allowedHeaders, ", ")
	exposeHeaders := strings.Join(cfg.Expose, ", ")

	// Pre-validate configuration for security
	hasWildcard := false
	for _, origin := range cfg.Origins {
		if origin == "*" {
			hasWildcard = true
			break
		}
	}

	// Security check: wildcard with credentials is not allowed
	if hasWildcard && cfg.Credentials {
		panic("CORS: cannot use wildcard origin (*) with credentials=true for security reasons")
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			origin := c.Request().Header.Get("Origin")

			// Determine allowed origin for this request
			var allowedOrigin string
			if len(cfg.Origins) > 0 {
				if hasWildcard {
					allowedOrigin = "*"
				} else if origin != "" && origin != "null" {
					// Validate origin against allowed list
					for _, allowed := range cfg.Origins {
						if origin == allowed {
							allowedOrigin = origin
							break
						}
					}
				}
			}

			// Set CORS headers
			if allowedOrigin != "" {
				c.Header("Access-Control-Allow-Origin", allowedOrigin)
			}
			if cfg.Credentials && allowedOrigin != "*" {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
			if exposeHeaders != "" {
				c.Header("Access-Control-Expose-Headers", exposeHeaders)
			}

			// Add security headers
			c.Header("X-Content-Type-Options", "nosniff")
			c.Header("X-Frame-Options", "DENY")

			if c.Method() == http.MethodOptions {
				// Only treat as preflight if Access-Control-Request-Method present
				requestMethod := c.Request().Header.Get("Access-Control-Request-Method")
				if requestMethod != "" {
					// Validate requested method
					methodAllowed := false
					for _, method := range allowedMethods {
						if requestMethod == method {
							methodAllowed = true
							break
						}
					}

					if !methodAllowed {
						return c.Status(http.StatusForbidden).String(http.StatusForbidden, "Method not allowed")
					}

					// Validate requested headers
					requestHeaders := c.Request().Header.Get("Access-Control-Request-Headers")
					if requestHeaders != "" && len(allowedHeaders) > 0 {
						requestedHeaders := strings.Split(strings.ToLower(requestHeaders), ",")
						for _, reqHeader := range requestedHeaders {
							reqHeader = strings.TrimSpace(reqHeader)
							headerAllowed := false
							for _, allowedHeader := range allowedHeaders {
								if strings.ToLower(reqHeader) == strings.ToLower(allowedHeader) {
									headerAllowed = true
									break
								}
							}
							if !headerAllowed {
								return c.Status(http.StatusForbidden).String(http.StatusForbidden, "Header not allowed")
							}
						}
					}

					if allowedMethodsStr != "" {
						c.Header("Access-Control-Allow-Methods", allowedMethodsStr)
					}
					if allowedHeadersStr != "" {
						c.Header("Access-Control-Allow-Headers", allowedHeadersStr)
					}
					if cfg.MaxAge > 0 {
						c.Header("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
					}
					return c.String(http.StatusNoContent, "")
				}
				return c.String(http.StatusOK, "")
			}
			return next(c)
		}
	}
}

// uniqOrDefault returns the input slice with duplicates removed, or the default
// if input is empty. Used internally to deduplicate CORS configuration values
// and provide sensible defaults.
//
// Example:
//
//	uniqOrDefault([]string{"GET", "POST", "GET"}, []string{"GET", "POST"})
//	// returns []string{"GET", "POST"}
func uniqOrDefault(v, def []string) []string {
	if len(v) == 0 {
		return def
	}
	m := map[string]struct{}{}
	res := make([]string, 0, len(v))
	for _, s := range v {
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			res = append(res, s)
		}
	}
	return res
}
