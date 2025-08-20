package app

import (
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/goflash/flash/v2/ctx"
	"github.com/julienschmidt/httprouter"
)

// Handler is the function signature for goflash route handlers and middleware (after composition).
// It receives a request context and returns an error, which is handled by the app's error handler.
type Handler func(ctx.Ctx) error

// Middleware transforms a Handler, enabling composition of cross-cutting concerns (e.g., logging, auth).
// Middleware is applied in the order registered, and can wrap or short-circuit the handler chain.
type Middleware func(Handler) Handler

// ErrorHandler handles errors returned from handlers.
// It is called after each request if the handler returns a non-nil error.
type ErrorHandler func(ctx.Ctx, error)

// Ctx is re-exported here for package-local convenience in tests and internal APIs.
// Note: External users should prefer flash.Ctx or ctx.Ctx.
type Ctx = ctx.Ctx

// DefaultApp is the main application/router for flash. It implements http.Handler and manages routing,
// middleware, error handling, and logger configuration.
//
// Performance note: sync.Pool is used for context reuse to minimize allocations
// in the hot path. This is safe because each request gets a fresh Ctx from the pool,
// and the pool is returned after the request is finished. This pattern is inspired
// by high-performance Go frameworks and is safe for concurrent use.
type DefaultApp struct {
	router     *httprouter.Router // underlying router
	middleware []Middleware       // global middleware
	pool       sync.Pool          // context pooling for allocation reduction
	OnError    ErrorHandler       // error handler
	NotFound   http.Handler       // handler for 404 Not Found
	MethodNA   http.Handler       // handler for 405 Method Not Allowed
	logger     *slog.Logger       // application logger
	healthPath string             // health check endpoint path
	healthFunc HealthCheckFunc    // custom health check function
}

// New creates a new DefaultApp with sensible defaults and returns it as the App interface.
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
// If not set, a default logger is used.
func (a *DefaultApp) SetLogger(l *slog.Logger) { a.logger = l }

// Logger returns the configured application logger, or slog.Default if none is set.
func (a *DefaultApp) Logger() *slog.Logger {
	if a.logger != nil {
		return a.logger
	}
	return slog.Default()
}

// Use registers global middleware, applied in the order added.
// Middleware is applied to all routes, in the order registered.
func (a *DefaultApp) Use(mw ...Middleware) {
	if len(mw) == 0 {
		return
	}
	a.middleware = append(a.middleware, mw...)
}

// ServeHTTP implements http.Handler by delegating to the internal router.
func (a *DefaultApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// Configuration setters to satisfy interface
func (a *DefaultApp) SetErrorHandler(h ErrorHandler)    { a.OnError = h }
func (a *DefaultApp) SetNotFoundHandler(h http.Handler) { a.NotFound = h }
func (a *DefaultApp) SetMethodNotAllowedHandler(h http.Handler) {
	a.MethodNA = h
}

// Health check functionality
func (a *DefaultApp) EnableHealthCheck(path string) {
	// If there's already a health check path, we need to remove the old route
	// Since httprouter doesn't have a direct "remove route" method,
	// we'll just update the path and the new route will override the old one
	a.healthPath = path
	a.GET(path, a.defaultHealthCheckHandler)
}

func (a *DefaultApp) SetHealthCheck(fn HealthCheckFunc) {
	a.healthFunc = fn
}

func (a *DefaultApp) HealthCheckPath() string {
	return a.healthPath
}

// defaultHealthCheckHandler is the default health check handler
func (a *DefaultApp) defaultHealthCheckHandler(c Ctx) error {
	status := "healthy"
	httpStatus := http.StatusOK

	// If a custom health check function is provided, use it
	if a.healthFunc != nil {
		if err := a.healthFunc(); err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			a.Logger().Error("health check failed", "error", err)
		}
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "goflash",
	}

	return c.Status(httpStatus).JSON(response)
}

// Getters to satisfy App interface without exposing fields when used as interface.
func (a *DefaultApp) ErrorHandler() ErrorHandler            { return a.OnError }
func (a *DefaultApp) NotFoundHandler() http.Handler         { return a.NotFound }
func (a *DefaultApp) MethodNotAllowedHandler() http.Handler { return a.MethodNA }
