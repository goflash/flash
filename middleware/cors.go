package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/goflash/flash/v2"
)

// CORSConfig holds configuration for the CORS middleware.
// Origins, Methods, and Headers control allowed cross-origin requests.
// Expose lists headers exposed to the browser. Credentials enables cookies. MaxAge sets preflight cache duration (seconds).
type CORSConfig struct {
	Origins     []string // allowed origins
	Methods     []string // allowed methods
	Headers     []string // allowed headers
	Expose      []string // headers to expose
	Credentials bool     // allow credentials
	MaxAge      int      // preflight cache duration (seconds)
}

// CORS returns middleware that sets CORS headers and handles preflight requests according to the provided config.
// For OPTIONS requests, it responds with allowed methods/headers and 204 No Content.
func CORS(cfg CORSConfig) flash.Middleware {
	allowedMethods := strings.Join(uniqOrDefault(cfg.Methods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}), ", ")
	allowedHeaders := strings.Join(cfg.Headers, ", ")
	exposeHeaders := strings.Join(cfg.Expose, ", ")
	origins := strings.Join(cfg.Origins, ", ")

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			if origins != "" {
				c.Header("Access-Control-Allow-Origin", origins)
			}
			if cfg.Credentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
			if exposeHeaders != "" {
				c.Header("Access-Control-Expose-Headers", exposeHeaders)
			}

			if c.Method() == http.MethodOptions {
				// Only treat as preflight if Access-Control-Request-Method present
				if c.Request().Header.Get("Access-Control-Request-Method") != "" {
					if allowedMethods != "" {
						c.Header("Access-Control-Allow-Methods", allowedMethods)
					}
					if allowedHeaders != "" {
						c.Header("Access-Control-Allow-Headers", allowedHeaders)
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
