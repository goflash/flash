package app

import "net/http"

// Group defines a group of routes with a common URL prefix and optional
// middleware. Groups allow modular organization of related routes and sharing of
// cross-cutting concerns such as authentication, logging, or rate limiting.
//
// A Group is typically created from an App (a *DefaultApp) via (*DefaultApp).Group.
// Nested groups may be created from an existing Group using (*Group).Group.
//
// Middleware Order:
//   - Global middleware (registered with App.Use) runs first in registration order
//   - Then parent group middleware, then child group middleware (outer -> inner)
//   - Then route-specific middleware (when passed during route registration)
//   - Finally the route handler
//
// Example (organization and middleware stacking):
//
//	a := app.New()
//	a.Use(Log) // global
//
//	api := a.Group("/api", Auth)           // /api with Auth
//	v1  := api.Group("/v1", Audit)         // /api/v1 with Auth then Audit
//	v1.GET("/users/:id", ShowUser, Trace)  // order: Log -> Auth -> Audit -> Trace -> ShowUser
//
// The final path for a group's route is joinPath(parent.prefix, group.prefix, routePath).
// Paths are normalized (e.g., duplicate slashes collapsed, trailing slash rules
// follow internal cleanPath/joinPath semantics). Route parameters (":id")
// defined in the path are available to handlers via c.Param("id").
type Group struct {
	app        *DefaultApp  // parent app
	prefix     string       // route prefix
	middleware []Middleware // group-level middleware
}

// Group creates a new route group with the given prefix and optional middleware.
// Routes registered on the group inherit the prefix and group middleware.
//
// The prefix is normalized and joined using internal helpers so that either
// absolute or relative segments work as expected (e.g., "/api" + "/v1" -> "/api/v1").
//
// Example:
//
//	api := a.Group("/api", Auth)
//	api.GET("/health", Health)
//	// registers handler at path "/api/health" with middleware: global -> Auth
//
// Example (adding middleware later):
//
//	api := a.Group("/api")
//	api.GET("/ping", Ping) // only global middleware applies
//	api.Use(Auth)
//	api.GET("/me", Me)    // now global -> Auth applies
func (a *DefaultApp) Group(prefix string, mw ...Middleware) *Group {
	return &Group{app: a, prefix: cleanPath(prefix), middleware: mw}
}

// Use adds middleware to the group. Middleware is applied in the order added.
//
// Example:
//
//	api := a.Group("/api")
//	api.Use(Auth, Audit)
//	api.GET("/users", ListUsers) // order: global -> Auth -> Audit -> handler
func (g *Group) Use(mw ...Middleware) { g.middleware = append(g.middleware, mw...) }

// Group creates a nested route group inheriting the parent's prefix and
// middleware. Additional middleware can be provided for the nested group.
//
// Example:
//
//	api := a.Group("/api", Auth)
//	v1 := api.Group("/v1", Audit)
//	v1.GET("/users", ListUsers) // path: /api/v1/users; order: global -> Auth -> Audit -> handler
//
// Example (deep nesting and route-specific middleware):
//
//	admin := v1.Group("/admin", AdminOnly)
//	admin.GET("/stats", Stats, Trace)
//	// order: global -> Auth -> Audit -> AdminOnly -> Trace -> Stats
func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	child := &Group{app: g.app, prefix: joinPath(g.prefix, prefix)}
	child.middleware = append(child.middleware, g.middleware...)
	if len(mw) > 0 {
		child.middleware = append(child.middleware, mw...)
	}
	return child
}

// handle registers a handler for the given HTTP method and relative path on the
// group. All group and route-specific middleware are applied.
//
// The final route path is the group prefix joined with the provided path using
// internal path normalization helpers. Users typically call the typed helpers
// (GET, POST, etc.) instead of handle directly.
//
// Example:
//
//	g.handle(http.MethodDelete, "/users/:id", DeleteUser)
//	// is equivalent to g.DELETE("/users/:id", DeleteUser)
func (g *Group) handle(method, p string, h Handler, mws ...Middleware) {
	all := append([]Middleware{}, g.middleware...)
	all = append(all, mws...)
	g.app.handle(method, joinPath(g.prefix, p), h, all...)
}

// GET registers a handler for HTTP GET requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
//
// Example:
//
//	api := a.Group("/api")
//	api.GET("/health", Health)
//
// Example (with params and route-specific middleware):
//
//	api.GET("/users/:id", ShowUser, Trace)
//	// handler sees c.Param("id"); order: global -> group -> Trace -> ShowUser
func (g *Group) GET(p string, h Handler, mws ...Middleware) { g.handle(http.MethodGet, p, h, mws...) }

// POST registers a handler for HTTP POST requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
// Commonly used for creating resources.
//
// Example:
//
//	api.POST("/users", CreateUser, CSRF)
//	// order: global -> group -> CSRF -> CreateUser
func (g *Group) POST(p string, h Handler, mws ...Middleware) { g.handle(http.MethodPost, p, h, mws...) }

// PUT registers a handler for HTTP PUT requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
// Typically used for full updates of resources.
//
// Example:
//
//	api.PUT("/users/:id", ReplaceUser)
func (g *Group) PUT(p string, h Handler, mws ...Middleware) { g.handle(http.MethodPut, p, h, mws...) }

// PATCH registers a handler for HTTP PATCH requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
// Typically used for partial updates of resources.
//
// Example:
//
//	api.PATCH("/users/:id", UpdateUserEmail)
func (g *Group) PATCH(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodPatch, p, h, mws...)
}

// DELETE registers a handler for HTTP DELETE requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
//
// Example:
//
//	api.DELETE("/users/:id", DeleteUser, Audit)
func (g *Group) DELETE(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodDelete, p, h, mws...)
}

// OPTIONS registers a handler for HTTP OPTIONS requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
// Useful for CORS preflight handling.
//
// Example:
//
//	api.OPTIONS("/users", Preflight)
func (g *Group) OPTIONS(p string, h Handler, mws ...Middleware) {
	g.handle(http.MethodOptions, p, h, mws...)
}

// HEAD registers a handler for HTTP HEAD requests on the group's prefix + path.
// Optionally accepts route-specific middleware.
// Mirrors GET semantics but returns no response body.
//
// Example:
//
//	api.HEAD("/health", HeadHealth)
func (g *Group) HEAD(p string, h Handler, mws ...Middleware) { g.handle(http.MethodHead, p, h, mws...) }
