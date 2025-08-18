package middleware

import (
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/ctx"
)

// Logger returns middleware that logs each request using slog, including method, path, status, duration, remote address, and user agent.
// The logger is taken from the request context or app, and can be enriched with a request ID if present.
func Logger() flash.Middleware {
	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			start := time.Now()
			err := next(c)
			dur := time.Since(start)

			status := c.StatusCode()
			if status == 0 {
				status = 200
			}

			ua, remote := "", ""
			if r := c.Request(); r != nil {
				ua = r.UserAgent()
				remote = r.RemoteAddr
			}

			l := ctx.LoggerFromContext(c.Context())

			attrs := []any{
				"method", c.Method(),
				"path", c.Path(),
				"route", c.Route(),
				"status", status,
				"duration_ms", float64(dur.Microseconds()) / 1000.0,
				"remote", remote,
				"user_agent", ua,
			}

			// optional enrichments
			if rid, ok := RequestIDFromContext(c.Context()); ok {
				attrs = append(attrs, "request_id", rid)
			}

			l.Info("request", attrs...)
			return err
		}
	}
}
