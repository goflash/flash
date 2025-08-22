package app

import (
	"log/slog"
	"net/http"
)

// App defines the public surface of the router/app, suitable for mocking.
// Implemented by *DefaultApp.
type App interface {
	// Middleware management
	Use(mw ...Middleware)

	// Route registration
	GET(path string, h Handler, mws ...Middleware)
	POST(path string, h Handler, mws ...Middleware)
	PUT(path string, h Handler, mws ...Middleware)
	PATCH(path string, h Handler, mws ...Middleware)
	DELETE(path string, h Handler, mws ...Middleware)
	OPTIONS(path string, h Handler, mws ...Middleware)
	HEAD(path string, h Handler, mws ...Middleware)
	ANY(path string, h Handler, mws ...Middleware)
	Handle(method, path string, h Handler, mws ...Middleware)

	// HTTP integration and mounting
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	HandleHTTP(method, path string, h http.Handler)
	Mount(path string, h http.Handler)
	Static(prefix, dir string)
	StaticDirs(prefix string, dirs ...string)

	// Grouping
	Group(prefix string, mws ...Middleware) *Group

	// Logging
	SetLogger(l *slog.Logger)
	Logger() *slog.Logger

	// Error/NotFound/MethodNotAllowed handlers
	SetErrorHandler(h ErrorHandler)
	SetNotFoundHandler(h http.Handler)
	SetMethodNotAllowedHandler(h http.Handler)

	// Getters for handlers (mirrors Set*). Useful when holding App as an interface.
	ErrorHandler() ErrorHandler
	NotFoundHandler() http.Handler
	MethodNotAllowedHandler() http.Handler
}
