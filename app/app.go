package app

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/goflash/flash/v2/ctx"
	"github.com/julienschmidt/httprouter"
)

// Handler is the function signature for goflash route handlers (and the output
// of composed middleware). It receives a request context and returns an error.
//
// Returning a non-nil error delegates to the App's ErrorHandler, allowing a
// single place to translate errors into HTTP responses and logs.
//
// Example:
//
//	func hello(c app.Ctx) error {
//		name := c.Param("name")
//		if name == "" {
//			return fmt.Errorf("missing name")
//		}
//		return c.String(http.StatusOK, "hello "+name)
//	}
type Handler func(ctx.Ctx) error

// Middleware transforms a Handler, enabling composition of cross-cutting
// concerns such as logging, authentication, rate limiting, etc.
//
// Middleware registered via Use is applied in the order added; route-specific
// middleware is applied after global middleware and before the route handler.
// A middleware can decide to short-circuit by returning without calling next.
//
// Example (logging middleware):
//
//	func Log(next app.Handler) app.Handler {
//		return func(c app.Ctx) error {
//			start := time.Now()
//			err := next(c)
//			logger := ctx.LoggerFromContext(c.Context())
//			logger.Info("handled",
//				"method", c.Method(),
//				"path", c.Path(),
//				"status", c.StatusCode(),
//				"dur", time.Since(start),
//			)
//			return err
//		}
//	}
type Middleware func(Handler) Handler

// ErrorHandler handles errors returned from handlers.
// It is called when a handler (or middleware) returns a non-nil error.
// Implementations should translate the error into an HTTP response and log it.
//
// Example:
//
//	func myErrorHandler(c app.Ctx, err error) {
//		logger := ctx.LoggerFromContext(c.Context())
//		logger.Error("request failed", "err", err)
//		_ = c.String(http.StatusInternalServerError, "internal error")
//	}
type ErrorHandler func(ctx.Ctx, error)

// Ctx is re-exported for package-local convenience in tests and internal APIs.
// External users can refer to this type as app.Ctx or ctx.Ctx.
type Ctx = ctx.Ctx

// DefaultApp is the main application/router for flash. It implements
// http.Handler, manages routing, middleware, error handling, and logger
// configuration.
//
// Performance note: a sync.Pool is used for context reuse to minimize
// allocations in the hot path. Each request acquires a fresh ctx.DefaultContext
// from the pool and returns it after completion. This pattern is safe for
// concurrent use and reduces GC pressure.
type DefaultApp struct {
	router     *httprouter.Router // underlying router
	middleware []Middleware       // global middleware
	pool       sync.Pool          // context pooling for allocation reduction
	OnError    ErrorHandler       // error handler
	NotFound   http.Handler       // handler for 404 Not Found
	MethodNA   http.Handler       // handler for 405 Method Not Allowed
	logger     *slog.Logger       // application logger
}

// New creates a new DefaultApp with sensible defaults and returns it as the App
// interface.
//
// Defaults include:
//   - JSON slog logger at info level to stdout
//   - 404 and 405 handlers wired to the internal router hooks
//   - MethodNotAllowed handling enabled on the router
//   - Context pooling for performance
//
// Example:
//
//	func main() {
//		a := app.New()
//		a.GET("/hello/:name", func(c app.Ctx) error {
//			return c.String(http.StatusOK, "hello "+c.Param("name"))
//		})
//		_ = http.ListenAndServe(":8080", a)
//	}
func New() App {
	app := &DefaultApp{
		router: httprouter.New(),
	}
	// Use sync.Pool to minimize allocations for context objects (hot path optimization)
	app.pool.New = func() any { return &ctx.DefaultContext{} }

	// Set up default handlers and logger
	app.router.HandleMethodNotAllowed = true
	app.SetErrorHandler(defaultErrorHandler)
	app.SetNotFoundHandler(http.NotFoundHandler())
	app.SetMethodNotAllowedHandler(methodNotAllowedHandler())
	app.SetLogger(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	app.router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.NotFoundHandler().ServeHTTP(w, r)
	})
	app.router.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.MethodNotAllowedHandler().ServeHTTP(w, r)
	})

	return app
}

// SetLogger sets the application logger used by middlewares and utilities.
// If not set, Logger() falls back to slog.Default().
//
// Example:
//
//	a.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
func (a *DefaultApp) SetLogger(l *slog.Logger) { a.logger = l }

// Logger returns the configured application logger, or slog.Default if none is set.
// Prefer enriching this logger with request-scoped fields in middleware using
// ctx.ContextWithLogger.
func (a *DefaultApp) Logger() *slog.Logger {
	if a.logger != nil {
		return a.logger
	}
	return slog.Default()
}

// Use registers global middleware, applied to all routes in the order added.
// Route-specific middleware passed at registration time is applied after global
// middleware.
//
// Example:
//
//	a.Use(Log, Recover)
//	a.GET("/", Home, Auth) // execution order: Log -> Recover -> Auth -> Home
func (a *DefaultApp) Use(mw ...Middleware) {
	if len(mw) == 0 {
		return
	}
	a.middleware = append(a.middleware, mw...)
}

// ServeHTTP implements http.Handler by delegating to the internal router.
// Typically you pass the App itself to http.ListenAndServe.
//
// Example:
//
//	_ = http.ListenAndServe(":8080", a)
func (a *DefaultApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// Configuration setters.
// These set the error, not found, and method-not-allowed handlers used by the app.
func (a *DefaultApp) SetErrorHandler(h ErrorHandler)    { a.OnError = h }
func (a *DefaultApp) SetNotFoundHandler(h http.Handler) { a.NotFound = h }
func (a *DefaultApp) SetMethodNotAllowedHandler(h http.Handler) {
	a.MethodNA = h
}

// Getters mirror the setters and are useful when holding App as an interface.
// They expose the currently configured handlers without exporting struct fields.
func (a *DefaultApp) ErrorHandler() ErrorHandler            { return a.OnError }
func (a *DefaultApp) NotFoundHandler() http.Handler         { return a.NotFound }
func (a *DefaultApp) MethodNotAllowedHandler() http.Handler { return a.MethodNA }
