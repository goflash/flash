<h1 align="center">
    <a href="https://goflash.dev">
        <picture>
            <img alt="GoFlash Framework" src="./public/images/logo-wide.png" alt="GoFlash Logo" width="600" />
        </picture>
    </a>
    <br />
    <a href="https://pkg.go.dev/github.com/goflash/flash/v2@v2.0.0-beta.4">
        <img src="https://pkg.go.dev/badge/github.com/goflash/flash.svg" alt="Go Reference">
    </a>
    <a href="https://goreportcard.com/report/github.com/goflash/flash">
        <img src="https://img.shields.io/badge/%F0%9F%93%9D%20Go%20Report-A%2B-75C46B?style=flat-square" alt="Go Report Card">
    </a>
    <a href="https://codecov.io/gh/goflash/flash">
        <img src="https://codecov.io/gh/goflash/flash/graph/badge.svg?token=VRHM48HJ5L" alt="Coverage">
    </a>
    <a href="https://github.com/goflash/flash/actions?query=workflow%3ATest">
        <img src="https://img.shields.io/github/actions/workflow/status/goflash/flash/test-coverage.yml?branch=main&label=%F0%9F%A7%AA%20Tests&style=flat-square&color=75C46B" alt="Tests">
    </a>
    <img src="https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=golang" alt="Go Version">
    <a href="https://docs.goflash.dev">
        <img src="https://img.shields.io/badge/%F0%9F%92%A1%20GoFlash-docs-00ACD7.svg?style=flat-square" alt="GoFlash Docs">
    </a>
    <img src="https://img.shields.io/badge/status-beta-yellow" alt="Status">
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
    <br>
    <div style="text-align:center">
      <a href="https://discord.gg/QHhGHtjjQG">
        <img src="https://dcbadge.limes.pink/api/server/https://discord.gg/QHhGHtjjQG" alt="Discord">
      </a>
    </div>
</h1>

<p align="center">
    <em>
        <b>Flash</b> is a lean web framework inspired by Gin and Fiber, combining their best. 
        Built on the standard <code>net/http</code>. 
        <br>
        It prioritizes developer speed and runtime performance - with a <b>tiny, tested and stable API</b>, 
        clean ergonomics, and near‑zero allocations in hot paths.
        <br>
        Ship features fast without sacrificing reliability.
    </em>
</p>



---

## Quick Start

```go
package main

import (
    "log"
    "net/http"

    "github.com/goflash/flash/v2"
    "github.com/goflash/flash/v2/middleware"
)

func main() {
    app := flash.New()
    app.Use(middleware.Recover(), middleware.Logger())

    // Easiest endpoint
    app.ANY("/ping", func(c flash.Ctx) error {
        return c.String(http.StatusOK, "pong")
    })

    // Path param with JSON response
    app.GET("/hello/:name", func(c flash.Ctx) error {
        return c.JSON(map[string]any{"hello": c.Param("name")})
    })

    // And many other possibilities without compromise on speed.

    log.Fatal(http.ListenAndServe(":8080", app))
}
```

> More examples 📁: Browse runnable examples in the separate repo: [goflash/examples](https://github.com/goflash/examples)

---

## Type-Safe Parameter Helpers

GoFlash provides convenient type-safe helpers for parsing query and path parameters, eliminating the need for manual string parsing and error handling:

```go
app.GET("/users/:id", func(c *flash.Ctx) error {
    // Type-safe path parameter parsing with defaults
    userID := c.ParamInt("id", 0)
    if userID == 0 {
        return c.JSON(map[string]string{"error": "invalid user ID"})
    }
    
    // Type-safe query parameter parsing
    page := c.QueryInt("page", 1)
    perPage := c.QueryInt("per_page", 10)
    includeDetails := c.QueryBool("details", false)
    
    return c.JSON(map[string]interface{}{
        "user_id": userID,
        "page": page,
        "per_page": perPage,
        "include_details": includeDetails,
    })
})
```

### Available Helpers

**Query Parameters:**
- `QueryInt(key, default)` - Parse as int
- `QueryInt64(key, default)` - Parse as int64  
- `QueryFloat64(key, default)` - Parse as float64
- `QueryBool(key, default)` - Parse as bool (accepts "true", "1", "yes", "on")
- `QueryUint(key, default)` - Parse as uint
- `QueryUint64(key, default)` - Parse as uint64

**Path Parameters:**
- `ParamInt(name, default)` - Parse as int
- `ParamInt64(name, default)` - Parse as int64
- `ParamFloat64(name, default)` - Parse as float64
- `ParamUint(name, default)` - Parse as uint
- `ParamUint64(name, default)` - Parse as uint64

All helpers return the default value if the parameter is missing or invalid, making your code more robust and concise.

---

## Philosophy & Overview
    
    // Type-safe query parameter parsing
    page := c.QueryInt("page", 1)
    perPage := c.QueryInt("per_page", 10)
    includeDetails := c.QueryBool("details", false)
    
    return c.JSON(map[string]interface{}{
        "user_id": userID,
        "page": page,
        "per_page": perPage,
        "include_details": includeDetails,
    })
})
```

### Available Helpers

**Query Parameters:**
- `QueryInt(key, default)` - Parse as int
- `QueryInt64(key, default)` - Parse as int64  
- `QueryFloat64(key, default)` - Parse as float64
- `QueryBool(key, default)` - Parse as bool (accepts "true", "1", "yes", "on")
- `QueryUint(key, default)` - Parse as uint
- `QueryUint64(key, default)` - Parse as uint64

**Path Parameters:**
- `ParamInt(name, default)` - Parse as int
- `ParamInt64(name, default)` - Parse as int64
- `ParamFloat64(name, default)` - Parse as float64
- `ParamUint(name, default)` - Parse as uint
- `ParamUint64(name, default)` - Parse as uint64

All helpers return the default value if the parameter is missing or invalid, making your code more robust and concise.

---

## Type-Safe Parameter Helpers

GoFlash provides convenient type-safe helpers for parsing query and path parameters, eliminating the need for manual string parsing and error handling:

```go
app.GET("/users/:id", func(c *flash.Ctx) error {
    // Type-safe path parameter parsing with defaults
    userID := c.ParamInt("id", 0)
    if userID == 0 {
        return c.JSON(map[string]string{"error": "invalid user ID"})
    }
    
    // Type-safe query parameter parsing
    page := c.QueryInt("page", 1)
    perPage := c.QueryInt("per_page", 10)
    includeDetails := c.QueryBool("details", false)
    
    return c.JSON(map[string]interface{}{
        "user_id": userID,
        "page": page,
        "per_page": perPage,
        "include_details": includeDetails,
    })
})
```

### Available Helpers

**Query Parameters:**
- `QueryInt(key, default)` - Parse as int
- `QueryInt64(key, default)` - Parse as int64  
- `QueryFloat64(key, default)` - Parse as float64
- `QueryBool(key, default)` - Parse as bool (accepts "true", "1", "yes", "on")
- `QueryUint(key, default)` - Parse as uint
- `QueryUint64(key, default)` - Parse as uint64

**Path Parameters:**
- `ParamInt(name, default)` - Parse as int
- `ParamInt64(name, default)` - Parse as int64
- `ParamFloat64(name, default)` - Parse as float64
- `ParamUint(name, default)` - Parse as uint
- `ParamUint64(name, default)` - Parse as uint64

All helpers return the default value if the parameter is missing or invalid, making your code more robust and concise.

---

## Overview
>>>>>>> 98d11f1 (feat: add type-safe query and path parameter helpers)

- **🎯 Purpose:** Productive HTTP framework with a tiny, composable core and batteries-included middlewares.
- **📐 Philosophy:** <u>Standard library first</u>, high performance without gimmicks, small API surface.
- **👥 Who is it for:** Teams that need Gin-like safety and net/http compatibility with Fiber-like ergonomics.
- **🧩 API:** Clean, minimal, and ergonomic—`flash.New()`, `flash.Ctx`, and composable middleware.
- **🔗 Interop & compatibility:** 100% net/http, HTTP/2-ready; mount any `http.Handler`, and `App` is an `http.Handler`.
- **🔌 Extensibility:** Add your own middleware, plug in any logger (slog, zap, zerolog), and compose freely.
- **🚀 Modern Go:** Designed for Go 1.22+, leverages context, slog, and best practices for performance and safety.
- **🛡️ Security:** Safe defaults; optional CSRF, timeouts, rate limiting,  session hardening via middleware and many more.
- **🛠️ Support:** Works with standard tooling (net/http, HTTP/2, pprof, Prometheus, OpenTelemetry, etc).
- **🧭 Scope:** Minimal core by design; advanced patterns live in middleware and the examples repository.

---

## Features

<<<<<<< HEAD
| Feature                    | Description & Rationale                                                                                                                                           |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **net/http compatible**    | `App` implements `http.Handler` for seamless integration with Go’s ecosystem and HTTP/2 readiness.                                                                |
| **Fast routing**           | High-performance router (httprouter): supports all HTTP verbs, route groups, and middleware composition.                                                          |
| **Ergonomic context**      | `flash.Ctx` provides clean helpers: `Param`, `Query`, typed `ParamInt/Bool/...`, `BindJSON`, `JSON`, `String`.                                                    |
| **Composable middleware**  | Global and per-route middleware, inspired by Gin/Fiber, for logging, recovery, CORS, and more.                                                                    |
| **Built-in middleware**    | Logger, Recover, CORS, Timeout, OpenTelemetry, Sessions, Gzip, Request ID, Rate Limit, Buffer.                                                                    |
| **Validation helpers**     | Integrated with go-playground/validator for robust request validation and field error mapping.                                                                    |
| **Static files**           | Serve static assets with `App.Static` or multiple folders with `App.StaticDirs` (first match wins).                                                               |
| **Hooks & error handling** | Custom `OnError`, `NotFound`, and `MethodNA` for full control over error and 404/405 responses.                                                                   |
| **Mounting/Interop**       | Mount any `http.Handler` or ServeMux; easy migration and integration with legacy or third-party code.                                                             |
| **Pluggable logging**      | Use any slog-compatible logger (slog, zap, zerolog); logger is injected into request context.                                                                     |
| **Observability**          | Built-in OpenTelemetry tracing middleware for distributed tracing and metrics.                                                                                    |
| **Session management**     | In-memory sessions with cookie/header ID; extensible for custom stores.                                                                                           |
| **Performance**            | Pooled buffers, precomputed Content-Length, pooled gzip writers, and efficient write buffering.                                                                   |
| **Extensible**             | Add your own middleware, context helpers, or validation logic; batteries-included but not batteries-opinionated.                                                  |
| **Modern Go**              | Designed for Go 1.22+, leverages context, slog, and idiomatic error handling.                                                                                     |
| **Examples**               | Real-world, runnable examples for features like cookies, templates, WebSockets, shutdown, and more (see [goflash/examples](https://github.com/goflash/examples)). |
=======
| Feature                    | Description & Rationale                                                                                          |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| **net/http compatible**    | `App` implements `http.Handler` for seamless integration with Go’s ecosystem and HTTP/2 readiness.               |
| **Fast routing**           | High-performance router (httprouter): supports all HTTP verbs, route groups, and middleware composition.         |
| **Ergonomic context**      | `flash.Ctx` provides clean helpers: `Param`, `Query`, `BindJSON`, `JSON`, `String`, `Header`, etc.               |
| **Composable middleware**  | Global and per-route middleware, inspired by Gin/Fiber, for logging, recovery, CORS, and more.                   |
| **Built-in middleware**    | Logger, Recover, CORS, Timeout, OpenTelemetry, Sessions, Gzip, Request ID, Rate Limit, Buffer.                   |
| **Validation helpers**     | Integrated with go-playground/validator for robust request validation and field error mapping.                   |
| **Static files**           | Serve static assets with `App.Static` or multiple folders with `App.StaticDirs` (first match wins).              |
| **Hooks & error handling** | Custom `OnError`, `NotFound`, and `MethodNA` for full control over error and 404/405 responses.                  |
| **Mounting/Interop**       | Mount any `http.Handler` or ServeMux; easy migration and integration with legacy or third-party code.            |
| **Pluggable logging**      | Use any slog-compatible logger (slog, zap, zerolog); logger is injected into request context.                    |
| **Observability**          | Built-in OpenTelemetry tracing middleware for distributed tracing and metrics.                                   |
| **Session management**     | In-memory sessions with cookie/header ID; extensible for custom stores.                                          |
| **Performance**            | Pooled buffers, precomputed Content-Length, pooled gzip writers, and efficient write buffering.                  |
| **Extensible**             | Add your own middleware, context helpers, or validation logic; batteries-included but not batteries-opinionated. |
| **Modern Go**              | Designed for Go 1.22+, leverages context, slog, and idiomatic error handling.                                    |
| **Examples**               | Real-world, runnable examples for all features: cookies, templates, WebSockets, shutdown, and more.              |
| **Type-safe parameters**   | Convenient helpers for parsing query and path parameters: `QueryInt()`, `QueryBool()`, `ParamInt()`, etc.        |

> Tip 💡: Every feature is included to maximize developer productivity, safety, and performance, while keeping the API minimal and explicit. No hidden magic, no global state, no surprises.
>>>>>>> 98d11f1 (feat: add type-safe query and path parameter helpers)

### Performance highlights

- Pooled JSON buffers for minimal allocations and fast serialization.
- Precomputed Content-Length for JSON, String, and Send responses (avoids chunked encoding, improves client performance).
- Pooled gzip writers per compression level for efficient, low-GC response compression.
- Optional write Buffer middleware: reduces syscalls, sets Content-Length, and auto-streams large payloads.
- Request context pooling: reuses context objects to minimize GC pressure and latency.
- Minimal allocations in routing and context handling (leverages httprouter and custom context pooling).
- Fast middleware chain: zero reflection, no global state, and no hidden allocations.
- All features are opt-in: no performance penalty for unused middleware or helpers.

---

## Why `GoFlash` vs Gin/Fiber/Others

GoFlash is designed to combine the best of Gin and Fiber, while addressing their key limitations for modern, professional Go development:

| Capability                  | Gin           | Fiber                 | GoFlash                               |
| --------------------------- | ------------- | --------------------- | ------------------------------------- |
| net/http compatible         | ✅ Yes         | ❌ No (fasthttp)       | ✅ Yes                                 |
| HTTP/2 readiness            | ✅ Yes         | ❌ No (adapter needed) | ✅ Yes                                 |
| Fast routing                | ✅ Yes         | ✅ Yes                 | ✅ Yes (httprouter)                    |
| Friendly context helpers    | ✅ Yes         | ✅ Yes                 | ✅ Yes (small API)                     |
| Middleware chaining         | ✅ Yes         | ✅ Yes                 | ✅ Yes (global + per-route)            |
| Route groups                | ✅ Yes         | ✅ Yes                 | ✅ Yes                                 |
| Validation helpers          | ☑️ Indirect    | ☑️ Indirect            | ✅ Yes (validator integrations)        |
| Tracing (OTel)              | ☑️ Via contrib | ☑️ Via contrib         | ✅ Built-in middleware                 |
| Sessions                    | ☑️ Via contrib | ☑️ Via contrib         | ✅ Built-in (in-memory, cookie/header) |
| Gzip/Request ID/Rate Limit  | ☑️ Via contrib | ☑️ Via contrib         | ✅ Built-in                            |
| Works with std http.Handler | ✅ Yes         | ❌ No                  | ✅ Yes (Mount/HandleHTTP)              |
| Graceful shutdown           | ✅ Yes         | ☑️ Partial             | ✅ Yes (context-aware)                 |
| HTTP/3 future-proof         | ✅ Yes         | ❌ No                  | ✅ Yes                                 |
| Extensible core             | ✅ Yes         | ✅ Yes                 | ✅ Yes (modular, opt-in)               |
| Modern Go idioms            | ☑️ Partial     | ☑️ Partial             | ✅ Yes (Go 1.22+, slog, context)       |

### Key differences and rationale

- **Standard library compatibility:** GoFlash is 100% net/http, so you get HTTP/2+, context cancellation, and all Go ecosystem tools out of the box—no adapters, no surprises.
- **Performance without trade-offs:** Like Fiber, GoFlash uses pooling and zero-allocation patterns, but never sacrifices reliability or compatibility. You get near-Fiber speed with Gin-level safety.
- **Minimal, ergonomic API:** Inspired by Fiber’s expressiveness and Gin’s clarity, GoFlash offers a small, explicit API—no magic, no global state, no hidden costs.
- **Batteries-included, but modular:** All common middleware (logging, recovery, CORS, tracing, sessions, gzip, rate limit, buffer) are built-in and opt-in. You only pay for what you use.
- **Observability and production readiness:** OpenTelemetry tracing, structured logging, and context helpers are first-class, not afterthoughts. Graceful shutdown and error handling are built-in.
- **Extensible and future-proof:** Designed for microservices, monoliths, and serverless. Clean project structure, easy to add your own middleware, and ready for new Go features (e.g., generics, slog).
- **Professional developer experience:** Clear docs, real-world examples, and a focus on explicitness and safety. No hidden magic, no global state, and no “gotchas” for teams scaling up.

---

## Install

```bash
go get github.com/goflash/flash/v2
```

---

## Core Concepts

### Routing

- Register routes with methods or `ANY()`. Group routes with shared prefix and middleware. Nested groups are supported and inherit parent prefix and middleware.
- Custom methods: use `Handle(method, path, handler)` for non-standard verbs.
- Mount net/http handlers with `Mount` or `HandleHTTP`.

#### Pattern reference

Routing patterns and behavior follow julienschmidt/httprouter. See:

- Named params: <https://github.com/julienschmidt/httprouter#named-parameters>
- Catch‑all (trailing wildcard): <https://github.com/julienschmidt/httprouter#catch-all-parameters>
- Trailing slash redirect rules: <https://github.com/julienschmidt/httprouter?tab=readme-ov-file#features>
- Automatic OPTIONS and Method Not Allowed: <https://github.com/julienschmidt/httprouter?tab=readme-ov-file#features>

### Context (Ctx)

flash.Ctx is a thin, pooled wrapper around http.ResponseWriter and *http.Request, designed for both ergonomics and performance. All helpers are explicit, chainable where appropriate, and safe for high-concurrency use.

<<<<<<< HEAD
| Method                                        | Purpose / Rationale                                                                                |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `Request()`                                   | Returns the underlying *http.Request for advanced/interop use.                                     |
| `SetRequest(r)`                               | Replace the request (e.g., to propagate context or swap in a new request).                         |
| `ResponseWriter()`                            | Returns the underlying http.ResponseWriter for low-level control.                                  |
| `SetResponseWriter(w)`                        | Replace the underlying http.ResponseWriter (e.g., for gzip or buffer middleware).                  |
| `WroteHeader()`                               | Reports whether the response header has been written.                                              |
| `Context()`                                   | Returns the request context for cancellation, deadlines, tracing, etc.                             |
| `Set(key, value)`                             | Store a value on the request context (clones request with context.WithValue).                      |
| `Get(key [,def])`                             | Retrieve a value from the request context; returns def if provided and missing, else nil.          |
| `Method()`                                    | Returns the HTTP method (GET, POST, etc).                                                          |
| `Path()`                                      | Returns the request URL path.                                                                      |
| `Route()`                                     | Returns the matched route pattern (e.g., `/users/:id`).                                            |
| `Param(name)`                                 | Returns a path parameter by name.                                                                  |
| `Query(key)`                                  | Returns a query string parameter by key.                                                           |
| `ParamInt/Int64/Uint/Float64/Bool(name, def)` | Typed path params with sensible defaults when missing/invalid.                                     |
| `QueryInt/Int64/Uint/Float64/Bool(key, def)`  | Typed query params with sensible defaults when missing/invalid.                                    |
| `Status(code)`                                | Sets the response status code (chainable, does not write header yet).                              |
| `StatusCode()`                                | Returns the status code that will be written (or 200 if not set yet).                              |
| `Header(key, value)`                          | Sets a response header.                                                                            |
| `SetJSONEscapeHTML(bool)`                     | Controls whether JSON responses escape HTML (default true, for XSS safety).                        |
| `JSON(v)`                                     | Writes a value as JSON, sets Content-Type/Length, and status (uses pooled buffer for performance). |
| `String(status, body)`                        | Writes a plain text response with status and body.                                                 |
| `Send(status, type, []byte)`                  | Writes raw bytes with status and content type.                                                     |
| `BindJSON(&v)`                                | Strictly decodes request body JSON into v (unknown fields rejected, closes body).                  |
| `Finish()`                                    | Finalizes the context (reserved for future buffer reuse, currently a no-op).                       |
| `Reset(w, r, ps, route)`                      | Internal: resets the context for pooling (not for user code).                                      |

> All methods are designed for explicitness, safety, and performance. You always have access to the underlying http types for advanced use, but the ergonomic helpers cover 99% of use cases.

#### Typed query and path parameters

Avoid repetitive `strconv` calls with typed helpers that return a parsed value or a default.

### Mounting/Interop

GoFlash is designed for seamless interoperability with the entire Go HTTP ecosystem. You can mount any `http.Handler`, `http.ServeMux`, or compatible router directly into your GoFlash app, making it easy to:

- Incrementally migrate legacy net/http codebases
- Integrate third-party routers, middleware, or microservices
- Share routes and handlers between GoFlash and standard library servers

#### Mounting http.Handler or ServeMux

Use `app.Mount(prefix, handler)` to attach any `http.Handler` (including `http.ServeMux`, other frameworks, or legacy code) under a path prefix. All requests matching the prefix are routed to the mounted handler, with the prefix stripped from the request URL (like Gin's `Group` or Fiber's `Mount`).

```go
// Mount a legacy net/http mux under /api/
mux := http.NewServeMux()
mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("legacy users"))
})
app.Mount("/api/", mux)
```

#### Mounting a single http.Handler on a route

Use `app.HandleHTTP(method, path, handler)` to register a single `http.Handler` for a specific method and path. This is ideal for integrating existing handlers or third-party libraries that expect net/http signatures.

```go
// Mount a single handler for GET /status
app.HandleHTTP("GET", "/status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("ok"))
}))
```

#### Migration and Interop Patterns

- **Incremental migration:** Start by mounting your existing `http.ServeMux` or legacy handlers, then gradually move routes to GoFlash for improved ergonomics and middleware support.
- **Third-party integration:** Use `Mount` or `HandleHTTP` to plug in routers or handlers from other libraries (e.g., Prometheus, pprof, grpc-gateway) without adapters.
- **Full net/http compatibility:** GoFlash apps are themselves `http.Handler`, so you can embed them in other servers, reverse proxies, or test harnesses.

#### Advanced: Composing with net/http

You can use GoFlash as a sub-router in a larger net/http application, or vice versa:

```go
// Use GoFlash as a sub-router in a standard net/http mux
app := flash.New()
// ...register routes...
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", app))
log.Fatal(http.ListenAndServe(":8080", mux))
```

> GoFlash is designed for zero-friction interop: no adapters, no wrappers, just standard Go interfaces. This makes it ideal for gradual adoption, microservices, and complex architectures.

### Logging

GoFlash uses Go's standard slog for framework logging and supports pluggable handlers. See the examples repository for end-to-end logging setups.

---

## Performance Notes

- JSON/String/Send set Content-Length when possible to avoid chunked responses where not needed.
- JSON uses a pooled buffer to minimize allocations; disable HTML escaping via `SetJSONEscapeHTML(false)` when safe.
- Gzip writers are pooled per compression level.
- Buffer middleware reduces syscalls by buffering responses and auto-switches to streaming when exceeding `MaxSize`.
- Request ID is available on the request context for low-overhead correlation in logs.

> For APIs with small/medium payloads, combining `Buffer`, `Gzip`, and precomputed `Content-Length` yields excellent performance with low GC pressure.

---

## Examples

All runnable examples live in a separate repository:

- Repository: [goflash/examples](https://github.com/goflash/examples)
- Explore topics like cookies, sessions, CSRF, templates, WebSockets, graceful shutdown, OpenTelemetry, and more.

To run an example, clone that repo and run it from its folder (many are standalone `go run .`).

---

## Benchmarks

We benchmarked GoFlash against Gin and Fiber across a representative set of scenarios:

1. Simple ping/pong endpoint
2. Reading a URL path parameter
3. Writing to and reading from request context
4. JSON binding with validation
5. Trailing-wildcard route parsing
6. Basic route group
7. Route groups nested 10 levels deep
8. Single middleware
9. Chain of 10 middlewares

Environment and methodology:

- Hardware: Apple MacBook Pro (M3, 32 GB RAM)
- Load generator: wrk with 11 threads and 256 concurrent connections
- Each scenario uses functionally equivalent handlers, routing patterns, and middleware across frameworks
- Servers run with release/production settings where applicable
- Results are indicative; performance varies with workload, configuration, and environment

<!-- markdownlint-disable-next-line MD033 -->
<img src="./public/images/all_benchmarks.png" alt="GoFlash Benchmarks" />

For more details: <https://github.com/goflash/benchmarks>

---

## Contributing

We welcome issues and PRs! Please read [CONTRIBUTING.md](./CONTRIBUTING.md).

---

<div align="center">

**⭐ Star this repo if you find it useful!**

[![GitHub stars](https://img.shields.io/github/stars/goflash/flash?style=social)](https://github.com/goflash/flash/stargazers)

---

<small>

**📝 License**: MIT | **📧 Support**: [Create an Issue]([../../issues](https://github.com/goflash/flash/issues))

Battle tested in private productions.
<br/> Released with ❤️ for the Go community.

</small>

</div>
=======
| Method                       | Purpose / Rationale                                                                                |
| ---------------------------- | -------------------------------------------------------------------------------------------------- |
| `Request()`                  | Returns the underlying *http.Request for advanced/interop use.                                     |
| `SetRequest(r)`              | Replace the request (e.g., to propagate context or swap in a new request).                         |
| `ResponseWriter()`           | Returns the underlying http.ResponseWriter for low-level control.                                  |
| `SetResponseWriter(w)`       | Replace the underlying http.ResponseWriter (e.g., for gzip or buffer middleware).                  |
| `WroteHeader()`              | Reports whether the response header has been written.                                              |
| `Context()`                  | Returns the request context for cancellation, deadlines, tracing, etc.                             |
| `Set(key, value)`            | Store a value on the request context (clones request with context.WithValue).                      |
| `Get(key [,def])`            | Retrieve a value from the request context; returns def if provided and missing, else nil.          |
| `Method()`                   | Returns the HTTP method (GET, POST, etc).                                                          |
| `Path()`                     | Returns the request URL path.                                                                      |
| `Route()`                    | Returns the matched route pattern (e.g., `/users/:id`).                                            |
| `Param(name)`                | Returns a path parameter by name.                                                                  |
| `Query(key)`                 | Returns a query string parameter by key.                                                           |
| `Status(code)`               | Sets the response status code (chainable, does not write header yet).                              |
| `StatusCode()`               | Returns the status code that will be written (or 200 if not set yet).                              |
| `Header(key, value)`         | Sets a response header.                                                                            |
| `SetJSONEscapeHTML(bool)`    | Controls whether JSON responses escape HTML (default true, for XSS safety).                        |
| `JSON(v)`                    | Writes a value as JSON, sets Content-Type/Length, and status (uses pooled buffer for performance). |
| `String(status, body)`       | Writes a plain text response with status and body.                                                 |
| `Send(status, type, []byte)` | Writes raw bytes with status and content type.                                                     |
| `BindJSON(&v)`               | Strictly decodes request body JSON into v (unknown fields rejected, closes body).                  |
| `Finish()`
>>>>>>> 98d11f1 (feat: add type-safe query and path parameter helpers)
