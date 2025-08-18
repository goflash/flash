package flash

import (
	"github.com/goflash/flash/v1/app"
	"github.com/goflash/flash/v1/ctx"
)

// Group is a route group for organizing routes. Re-exported from app.Group for convenience.
type Group = app.Group

// App is the main application/router. Implements http.Handler. Re-exported from app.App.
type App = app.App

// Handler is the function signature for goflash route handlers and middleware (after composition).
// Re-exported from app.Handler.
type Handler = app.Handler

// Middleware transforms a Handler, enabling composition (e.g., logging, auth).
// Re-exported from app.Middleware.
type Middleware = app.Middleware

// ErrorHandler handles errors returned from handlers. Re-exported from app.ErrorHandler.
type ErrorHandler = app.ErrorHandler

// Ctx is the request context, re-exported for convenience.
type Ctx = ctx.Ctx

// New creates a new App with sensible defaults. Re-exported from app.New.
func New() *App { return app.New() }
