package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
	"go.uber.org/zap"
)

// Example-only slog handler backed by zap. In real apps, consider a maintained adapter.
type zapHandler struct{ z *zap.Logger }

func (h *zapHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *zapHandler) Handle(_ context.Context, r slog.Record) error {
	lvl := r.Level
	switch lvl {
	case slog.LevelDebug:
		h.z.Debug(r.Message)
	case slog.LevelInfo:
		h.z.Info(r.Message)
	case slog.LevelWarn:
		h.z.Warn(r.Message)
	case slog.LevelError:
		h.z.Error(r.Message)
	default:
		h.z.Info(r.Message)
	}
	return nil
}

func (h *zapHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *zapHandler) WithGroup(_ string) slog.Handler          { return h }

func main() {
	zl, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer zl.Sync()

	app := flash.New()

	adapter := &zapHandler{z: zl}
	app.SetLogger(slog.New(adapter))

	app.Use(mw.Logger())
	app.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "zap via slog adapter") })

	log.Fatal(http.ListenAndServe(":8080", app))
}
