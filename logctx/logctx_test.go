package logctx

import (
	"context"
	"log/slog"
	"testing"
)

func TestContextLoggerRoundTrip(t *testing.T) {
	ctx := context.Background()
	l := slog.Default()
	ctx = ContextWithLogger(ctx, l)
	if got := LoggerFromContext(ctx); got == nil {
		t.Fatalf("logger missing")
	}
}

func TestLoggerFromContextDefault(t *testing.T) {
	ctx := context.Background()
	if LoggerFromContext(ctx) == nil {
		t.Fatalf("expected default logger")
	}
}
