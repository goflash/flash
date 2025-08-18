package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/goflash/flash/v2"
)

// RequestIDConfig configures the RequestID middleware.
// Header sets the response header name (default: X-Request-ID).
type RequestIDConfig struct {
	Header string // response header name, default: X-Request-ID
}

type ridKey struct{}

// RequestID returns middleware that adds a unique request ID to each request/response.
// The request ID is set in the configured header and made available in the request context.
func RequestID(cfgs ...RequestIDConfig) flash.Middleware {
	cfg := RequestIDConfig{Header: "X-Request-ID"}
	if len(cfgs) > 0 && cfgs[0].Header != "" {
		cfg.Header = cfgs[0].Header
	}
	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			id := c.Request().Header.Get(cfg.Header)
			if id == "" {
				id = newID()
			}
			c.Header(cfg.Header, id)
			ctx := context.WithValue(c.Context(), ridKey{}, id)
			r := c.Request().WithContext(ctx)
			c.SetRequest(r)
			return next(c)
		}
	}
}

// RequestIDFromContext returns the request ID from the context, if available.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(ridKey{})
	if v == nil {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
