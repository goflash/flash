package flash

import (
	"github.com/goflash/flash/v2/app"
	"github.com/goflash/flash/v2/ctx"
)

// Group is a route group for organizing routes. Re-exported from app.Group for convenience.
type Group = app.Group

// App is the public interface of the application, re-exported for convenience.
type App = app.App

// DefaultApp is the default implementation (useful when asserting concrete type in tests).
type DefaultApp = app.DefaultApp

// Handler is the function signature for goflash route handlers and middleware (after composition).
// Re-exported from app.Handler.
type Handler = app.Handler

// Middleware transforms a Handler, enabling composition (e.g., logging, auth).
// Re-exported from app.Middleware.
type Middleware = app.Middleware

// ErrorHandler handles errors returned from handlers. Re-exported from app.ErrorHandler.
type ErrorHandler = app.ErrorHandler

// Ctx is the request context interface, re-exported for convenience.
type Ctx = ctx.Ctx

// DefaultContext is the concrete context implementation used by the framework.
type DefaultContext = ctx.DefaultContext

// New creates a new App with sensible defaults. Re-exported from app.New.
func New() App { return app.New() }
