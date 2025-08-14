package core

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// ContextWithLogger returns a new context carrying the provided slog.Logger.
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, l)
}

// LoggerFromContext returns a slog.Logger from context or slog.Default if none.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if v := ctx.Value(loggerContextKey{}); v != nil {
		if l, ok := v.(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
