package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/goflash/flash"
)

// TimeoutConfig configures the timeout middleware.
// Duration sets the timeout. OnTimeout is called when a timeout occurs. ErrorResponse can customize the timeout response.
type TimeoutConfig struct {
	Duration      time.Duration          // request timeout duration
	OnTimeout     func(*flash.Ctx)       // optional callback on timeout
	ErrorResponse func(*flash.Ctx) error // optional custom error response
}

// Timeout returns middleware that applies a timeout to the request context.
// If the handler does not complete within Duration, a 504 Gateway Timeout is returned.
func Timeout(cfg TimeoutConfig) flash.Middleware {
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			ctx, cancel := context.WithTimeout(c.Context(), cfg.Duration)
			defer cancel()
			r := c.Request().WithContext(ctx)
			c.SetRequest(r)

			done := make(chan error, 1)
			go func() {
				done <- next(c)
			}()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				if cfg.OnTimeout != nil {
					cfg.OnTimeout(c)
				}
				if cfg.ErrorResponse != nil {
					return cfg.ErrorResponse(c)
				}
				return c.String(http.StatusGatewayTimeout, http.StatusText(http.StatusGatewayTimeout))
			}
		}
	}
}
