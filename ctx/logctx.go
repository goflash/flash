package ctx

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// ContextWithLogger returns a new context carrying the provided slog.Logger.
//
// Attach a request-scoped logger in middleware and retrieve it later from the
// request context. When combined with ctx.Ctx, you can store the logger on the
// request using c.Set(key, value) or by replacing the underlying request with a
// derived context that includes the logger.
//
// Typical middleware usage:
//
//	func LoggingMiddleware(next flash.Handler) flash.Handler {
//		return func(c ctx.Ctx) error {
//			l := slog.Default().With("req_id", c.Get("req_id"))
//			r := c.Request().WithContext(ctx.ContextWithLogger(c.Context(), l))
//			c.SetRequest(r)
//			return next(c)
//		}
//	}
//
// In a handler:
//
//	func Show(c ctx.Ctx) error {
//		l := ctx.LoggerFromContext(c.Context())
//		l.Info("handling request", "path", c.Path())
//		return c.String(200, "ok")
//	}
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, l)
}

// LoggerFromContext returns a slog.Logger from the context, or slog.Default if
// none is found.
//
// This helper ensures handlers and middleware can always log even if a
// request-scoped logger was not injected. Prefer injecting a logger with
// ContextWithLogger so you can enrich it with request fields (request id, user
// id, route, etc.).
//
// Example:
//
//	l := ctx.LoggerFromContext(c.Context())
//	l.Info("user fetched", "user_id", id)
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if v := ctx.Value(loggerContextKey{}); v != nil {
		if l, ok := v.(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
