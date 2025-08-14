# Contributing to goflash

Thanks for your interest! This project keeps a minimal core and encourages optional middleware.

- Core: net/http compatible router, context, middleware chaining
- Optional: logging, recover, CORS, timeout, tracing, etc

Development

- Requires Go 1.22+
- Example: ./examples/

Coding style

- Prefer small, focused packages
- Avoid reflection on hot paths
- Use sync.Pool conservatively for request-local objects

Tests

- Please include unit tests for core functionality
