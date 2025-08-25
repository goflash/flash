package app

import (
	"net/http"

	"github.com/goflash/flash/v2/ctx"
	"github.com/julienschmidt/httprouter"
)

// GET registers a handler for HTTP GET requests on the given path.
// Optionally accepts route-specific middleware.
//
// Example:
//
//	a := app.New()
//	a.GET("/health", func(c app.Ctx) error { return c.String(http.StatusOK, "ok") })
//
// Example (with route params and middleware):
//
//	a.GET("/users/:id", ShowUser, Auth)
//	// order: global -> Auth -> ShowUser; handler sees c.Param("id")
func (a *DefaultApp) GET(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodGet, path, h, mws...)
}

// POST registers a handler for HTTP POST requests on the given path.
// Optionally accepts route-specific middleware.
// Commonly used for creating resources.
//
// Example:
//
//	a.POST("/users", CreateUser, CSRF)
func (a *DefaultApp) POST(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPost, path, h, mws...)
}

// PUT registers a handler for HTTP PUT requests on the given path.
// Optionally accepts route-specific middleware.
// Typically used for full resource replacement.
//
// Example:
//
//	a.PUT("/users/:id", ReplaceUser)
func (a *DefaultApp) PUT(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPut, path, h, mws...)
}

// PATCH registers a handler for HTTP PATCH requests on the given path.
// Optionally accepts route-specific middleware.
// Typically used for partial updates.
//
// Example:
//
//	a.PATCH("/users/:id", UpdateUserEmail)
func (a *DefaultApp) PATCH(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPatch, path, h, mws...)
}

// DELETE registers a handler for HTTP DELETE requests on the given path.
// Optionally accepts route-specific middleware.
//
// Example:
//
//	a.DELETE("/users/:id", DeleteUser, Audit)
func (a *DefaultApp) DELETE(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodDelete, path, h, mws...)
}

// OPTIONS registers a handler for HTTP OPTIONS requests on the given path.
// Optionally accepts route-specific middleware.
// Useful for CORS preflight handling.
//
// Example:
//
//	a.OPTIONS("/users", Preflight)
func (a *DefaultApp) OPTIONS(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodOptions, path, h, mws...)
}

// HEAD registers a handler for HTTP HEAD requests on the given path.
// Optionally accepts route-specific middleware.
// Mirrors GET semantics but does not write a response body.
//
// Example:
//
//	a.HEAD("/health", HeadHealth)
func (a *DefaultApp) HEAD(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodHead, path, h, mws...)
}

// ANY registers a handler for all common HTTP methods (GET, POST, PUT, PATCH,
// DELETE, OPTIONS, HEAD) on the given path.
// Optionally accepts route-specific middleware.
//
// Example:
//
//	a.ANY("/webhook", Webhook)
func (a *DefaultApp) ANY(path string, h Handler, mws ...Middleware) {
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead} {
		a.handle(m, path, h, mws...)
	}
}

// Handle registers a handler for a custom HTTP method on the given path.
// Optionally accepts route-specific middleware.
// Use this for less common methods (e.g., PROPFIND, REPORT) or extension
// methods used by specialized clients.
//
// Example:
//
//	a.Handle("REPORT", "/dav/resource", HandleReport)
func (a *DefaultApp) Handle(method, path string, h Handler, mws ...Middleware) {
	a.handle(method, path, h, mws...)
}

// handle is the internal route registration and handler composition method.
// It composes the middleware chain (route-specific then global), adapts the
// handler to the httprouter signature, injects the logger, and manages context
// pooling for allocation-free request handling.
//
// Middleware composition order:
//   - Route-specific middleware wraps the handler (right-to-left)
//   - Then global middleware wraps that (right-to-left)
//
// The resulting call order at runtime is: global (left-to-right) -> route (left-to-right) -> handler.
//
// Context lifecycle:
//   - Acquire a *ctx.DefaultContext from the pool
//   - Reset it with the incoming request/params and computed route pattern
//   - Call the composed handler
//   - On error, invoke the configured ErrorHandler
//   - Finish() and return the context to the pool
//
// Example (internal flow overview):
//
//	// Registration
//	a.GET("/users/:id", Show, Auth)
//
//	// Internally becomes something like:
//	// final := Global2(Global1(Auth(Show)))
//	// router.Handle("GET", "/users/:id", adapted(final))
func (a *DefaultApp) handle(method, path string, h Handler, mws ...Middleware) {
	// Compose middleware chain right-to-left for minimal allocations and call depth.
	// Route-specific middleware wraps the handler, then global middleware wraps that.
	// This is allocation-free: each layer is a direct function call, not a slice or struct.
	final := h
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	for i := len(a.middleware) - 1; i >= 0; i-- {
		final = a.middleware[i](final)
	}

	// Adapt to httprouter signature and manage context lifecycle.
	pattern := path
	a.router.Handle(method, path, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Inject app logger into request context for structured logging.
		r = r.WithContext(ctx.ContextWithLogger(r.Context(), a.Logger()))
		concrete := a.pool.Get().(*ctx.DefaultContext)
		concrete.Reset(w, r, ps, pattern)
		if err := final(concrete); err != nil {
			a.ErrorHandler()(concrete, err)
		}
		concrete.Finish()
		a.pool.Put(concrete)
	})
}
