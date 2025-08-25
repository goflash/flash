package app

import (
	"net/http"

	"github.com/goflash/flash/v2/ctx"
)

// defaultErrorHandler is the built-in error handler used by New() unless
// replaced via SetErrorHandler. It writes a generic 500 Internal Server Error
// if the response has not already started.
//
// Behavior:
//   - If the handler/middleware already wrote the header, this function does nothing
//     to avoid corrupting a streaming or partially-sent response.
//   - Otherwise, it writes status 500 with a plain text body of
//     http.StatusText(http.StatusInternalServerError).
//
// Applications typically provide their own ErrorHandler to log errors, map
// domain errors to HTTP status codes, and return structured JSON bodies.
//
// Example (custom error handler):
//
//	func myErrorHandler(c app.Ctx, err error) {
//		logger := ctx.LoggerFromContext(c.Context())
//		logger.Error("request failed", "err", err)
//		if c.WroteHeader() {
//			return
//		}
//		_ = c.JSON(map[string]any{"error": "internal"})
//	}
//
//	a := app.New()
//	a.SetErrorHandler(myErrorHandler)
func defaultErrorHandler(c ctx.Ctx, err error) {
	// if response already started, do nothing
	if c.WroteHeader() {
		return
	}
	_ = c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}

// methodNotAllowedHandler returns a handler for 405 Method Not Allowed responses.
// It is installed by New() and can be replaced via SetMethodNotAllowedHandler.
//
// The default behavior simply writes status 405 with a plain text body, without
// attempting content negotiation. Applications can swap this for a JSON or HTML
// variant, or for adding CORS/Allow headers as needed.
//
// Example (custom handler):
//
//	a.SetMethodNotAllowedHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		w.Header().Set("Content-Type", "application/json")
//		w.WriteHeader(http.StatusMethodNotAllowed)
//		_, _ = w.Write([]byte(`{"error":"method not allowed"}`))
//	}))
func methodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(http.StatusText(http.StatusMethodNotAllowed)))
	})
}
