package app

import (
	"net/http"

	"github.com/goflash/flash/ctx"
)

// defaultErrorHandler is the default error handler for goflash applications.
// It writes a 500 Internal Server Error if the response has not already started.
func defaultErrorHandler(c *ctx.Ctx, err error) {
	// if response already started, do nothing
	if c.WroteHeader() {
		return
	}
	_ = c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}

// methodNotAllowedHandler returns a handler for 405 Method Not Allowed responses.
func methodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(http.StatusText(http.StatusMethodNotAllowed)))
	})
}
