package core

import (
	"context"
	"log/slog"
	"testing"
)

func TestCoreLoggerContextRoundTripAndDefault(t *testing.T) {
	ctx := context.Background()
	if got := LoggerFromContext(ctx); got == nil {
		t.Fatalf("expected default logger, got nil")
	}
	l := slog.Default()
	ctx = ContextWithLogger(ctx, l)
	if got := LoggerFromContext(ctx); got != l {
		t.Fatalf("logger mismatch")
	}
}
