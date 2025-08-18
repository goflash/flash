package middleware

import (
	"net/http"

	"github.com/goflash/flash/v1"
)

// Recover returns middleware that recovers from panics in handlers and returns a 500 Internal Server Error.
// This prevents panics from crashing the server and provides a generic error response.
func Recover() flash.Middleware {
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) (err error) {
			defer func() {
				if r := recover(); r != nil {
					_ = c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
				}
			}()
			return next(c)
		}
	}
}
