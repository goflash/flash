<h1 align="center">
    <a href="https://goflash.dev">
        <picture>
            <img alt="GoFlash Framework" src="./public/images/logo-wide.png" alt="GoFlash Logo" width="600" />
        </picture>
    </a>
    <br />
    <a href="https://pkg.go.dev/github.com/goflash/flash/v2@v2.0.0-beta.8">
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
    <img src="https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=golang" alt="Go Version">
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

    log.Fatal(http.ListenAndServe(":8080", app))
}
```

More examples: [goflash/examples](https://github.com/goflash/examples)

## Features

- **net/http compatible** - Full compatibility with Go's standard library and HTTP/2
- **Fast routing** - High-performance routing with support for path parameters and route groups  
- **Ergonomic context** - Clean API with helpers for common operations
- **Composable middleware** - Built-in middleware for logging, recovery, CORS, sessions, and more
- **Static file serving** - Serve static assets with flexible configuration
- **Request binding** - Bind JSON, form, query, and path parameters to structs
- **Extensible** - Add custom middleware and integrate with any slog-compatible logger

---

## Middleware

Flash includes built-in middleware for common web application needs and supports external middleware packages.

### Core Middleware

| Middleware  | Purpose                                                                     |
| ----------- | --------------------------------------------------------------------------- |
| Buffer      | Response buffering to reduce syscalls and set Content-Length                |
| CORS        | Cross-origin resource sharing with configurable policies                    |
| CSRF        | Cross-site request forgery protection using double-submit cookies           |
| Logger      | Structured request logging with slog integration                            |
| RateLimit   | Rate limiting with multiple strategies (token bucket, sliding window, etc.) |
| Recover     | Panic recovery with configurable error responses                            |
| RequestID   | Request ID generation and correlation                                       |
| RequestSize | Request body size limiting for DoS protection                               |
| Session     | Session management with pluggable storage backends                          |
| Timeout     | Request timeout handling with graceful cancellation                         |

### External Middleware

| Package       | Description                                          | Repository                                                      |
| ------------- | ---------------------------------------------------- | --------------------------------------------------------------- |
| OpenTelemetry | Distributed tracing and metrics integration          | [goflash/otel](https://github.com/goflash/otel)                 |
| Validator     | Request validation with go-playground/validator      | [goflash/validator](https://github.com/goflash/validator)       |
| Compression   | Compression middleware for the GoFlash web framework | [goflash/compression](https://github.com/compression/validator) |

---

## Installation

```bash
go get github.com/goflash/flash/v2
```

---

## Core Concepts

### Routing

- Register routes with methods or `ANY()`. Group routes with shared prefix and middleware. Nested groups are supported and inherit parent prefix and middleware.
- Custom methods: use `Handle(method, path, handler)` for non-standard verbs.
- Mount net/http handlers with `Mount` or `HandleHTTP`.

#### Routing patterns reference

Routing patterns and behavior follow [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter).

### Context (Ctx)

`flash.Ctx` is a wrapper around `http.ResponseWriter` and `*http.Request` that provides convenient helpers for common operations:

- **Request/Response Access** - Direct access to underlying HTTP primitives
- **Path & Query Parameters** - Extract and parse URL parameters with type conversion
- **Request Binding** - Bind JSON, form, query, and path data to structs
- **Response Writing** - Send JSON, text, or raw responses with proper headers
- **Context Management** - Store and retrieve values in request context

For detailed method documentation, see the [Go package documentation](https://pkg.go.dev/github.com/goflash/flash/v2).

### Request Binding

Flash provides helpers to bind request data to structs using `json` tags:

- `BindJSON` - Bind JSON request body
- `BindForm` - Bind form data (URL-encoded or multipart)
- `BindQuery` - Bind query parameters
- `BindPath` - Bind path parameters
- `BindAny` - Bind from multiple sources with precedence: Path > Body > Query

```go
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

app.POST("/users/:id", func(c flash.Ctx) error {
    var user User
    if err := c.BindAny(&user); err != nil {
        return c.Status(400).JSON(map[string]string{"error": err.Error()})
    }
    return c.JSON(user)
})
```

### net/http Interoperability

Flash is fully compatible with the standard library. You can:

- Mount any `http.Handler` using `app.Mount(prefix, handler)`
- Register individual handlers with `app.HandleHTTP(method, path, handler)`
- Use Flash apps as `http.Handler` in other servers

```go
// Mount existing handler
mux := http.NewServeMux()
mux.HandleFunc("/users", userHandler)
app.Mount("/api/", mux)

// Use Flash in standard server
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", app))
```

---

## Examples

Runnable examples covering various use cases are available at [goflash/examples](https://github.com/goflash/examples).

For contrib-specific examples look at every specific middleware repository.

---

## Benchmarks

Flash is benchmarked against Gin and Fiber across common web application scenarios. Performance is competitive with other major Go web frameworks.

![GoFlash Benchmarks](./public/images/all_benchmarks.png)

Detailed benchmarks: [goflash/benchmarks](https://github.com/goflash/benchmarks)

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
