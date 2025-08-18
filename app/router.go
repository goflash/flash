package app

import (
	"net/http"

	"github.com/goflash/flash/v2/ctx"
	"github.com/julienschmidt/httprouter"
)

// GET registers a handler for HTTP GET requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) GET(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodGet, path, h, mws...)
}

// POST registers a handler for HTTP POST requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) POST(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPost, path, h, mws...)
}

// PUT registers a handler for HTTP PUT requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) PUT(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPut, path, h, mws...)
}

// PATCH registers a handler for HTTP PATCH requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) PATCH(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodPatch, path, h, mws...)
}

// DELETE registers a handler for HTTP DELETE requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) DELETE(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodDelete, path, h, mws...)
}

// OPTIONS registers a handler for HTTP OPTIONS requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) OPTIONS(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodOptions, path, h, mws...)
}

// HEAD registers a handler for HTTP HEAD requests on the given path.
// Optionally accepts route-specific middleware.
func (a *App) HEAD(path string, h Handler, mws ...Middleware) {
	a.handle(http.MethodHead, path, h, mws...)
}

// ANY registers a handler for all common HTTP methods (GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD)
// on the given path. Optionally accepts route-specific middleware.
func (a *App) ANY(path string, h Handler, mws ...Middleware) {
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead} {
		a.handle(m, path, h, mws...)
	}
}

// Handle registers a handler for a custom HTTP method on the given path.
// Optionally accepts route-specific middleware.
func (a *App) Handle(method, path string, h Handler, mws ...Middleware) {
	a.handle(method, path, h, mws...)
}

// handle is the internal route registration and handler composition method.
// It composes the middleware chain (route-specific then global), adapts the handler to the httprouter signature,
// injects the logger, and manages context pooling for allocation-free request handling.
func (a *App) handle(method, path string, h Handler, mws ...Middleware) {
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
		ctxFromPool := a.pool.Get().(*Ctx)
		ctxFromPool.Reset(w, r, ps, pattern)
		if err := final(ctxFromPool); err != nil {
			a.OnError(ctxFromPool, err)
		}
		ctxFromPool.Finish()
		a.pool.Put(ctxFromPool)
	})
}
