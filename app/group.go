package app

import "net/http"

// Group defines a group of routes with a common prefix and optional middleware.
// Groups allow for modular organization of related routes and shared middleware.
type Group struct {
	app        *DefaultApp  // parent app
	prefix     string       // route prefix
	middleware []Middleware // group-level middleware
}

// Group creates a new route group with the given prefix and optional middleware.
// Routes registered on the group will inherit the prefix and middleware.
func (a *DefaultApp) Group(prefix string, mw ...Middleware) *Group {
	return &Group{app: a, prefix: cleanPath(prefix), middleware: mw}
}

// Use adds middleware to the group. Middleware is applied in the order added.
func (g *Group) Use(mw ...Middleware) { g.middleware = append(g.middleware, mw...) }

// Group creates a nested route group inheriting the parent's prefix and middleware.
// Additional middleware can be provided for the nested group.
func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	child := &Group{app: g.app, prefix: joinPath(g.prefix, prefix)}
	child.middleware = append(child.middleware, g.middleware...)
	if len(mw) > 0 {
		child.middleware = append(child.middleware, mw...)
	}
	return child
}

// handle registers a handler for the given HTTP method and path on the group.
// All group and route-specific middleware are applied.
func (g *Group) handle(method, p string, h Handler, mws ...Middleware) {
	all := append([]Middleware{}, g.middleware...)
	all = append(all, mws...)
	g.app.handle(method, joinPath(g.prefix, p), h, all...)
}

// GET registers a handler for HTTP GET requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) GET(p string, h Handler, mws ...Middleware) { g.handle(http.MethodGet, p, h, mws...) }

// POST registers a handler for HTTP POST requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) POST(p string, h Handler, mws ...Middleware) { g.handle(http.MethodPost, p, h, mws...) }

// PUT registers a handler for HTTP PUT requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) PUT(p string, h Handler, mws ...Middleware) { g.handle(http.MethodPut, p, h, mws...) }

// PATCH registers a handler for HTTP PATCH requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) PATCH(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodPatch, p, h, mws...)
}

// DELETE registers a handler for HTTP DELETE requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) DELETE(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodDelete, p, h, mws...)
}

// OPTIONS registers a handler for HTTP OPTIONS requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) OPTIONS(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodOptions, p, h, mws...)
}

// HEAD registers a handler for HTTP HEAD requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
func (g *Group) HEAD(p string, h Handler, mws ...Middleware) { g.handle(http.MethodHead, p, h, mws...) }
