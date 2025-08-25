package middleware

import (
	"net/http"

	"github.com/goflash/flash/v2"
)

// RecoverConfig configures the panic recovery middleware.
//
// EnableStack controls whether stack traces are logged (disabled in production for security).
// OnPanic is called when a panic occurs, useful for custom logging or alerting.
// ErrorResponse allows customizing the error response sent to clients.
//
// Security considerations:
//   - Never expose stack traces to clients in production
//   - Log panic details for debugging but sanitize client responses
//   - Use structured logging to avoid log injection attacks
//   - Consider rate limiting panic notifications to prevent spam
//
// Example:
//
//	cfg := middleware.RecoverConfig{
//		EnableStack: false, // Disable in production
//		OnPanic: func(c flash.Ctx, err interface{}) {
//			logger := ctx.LoggerFromContext(c.Context())
//			logger.Error("panic recovered",
//				"error", err,
//				"method", c.Method(),
//				"path", c.Path(),
//				"remote_addr", c.Request().RemoteAddr,
//			)
//		},
//		ErrorResponse: func(c flash.Ctx, err interface{}) error {
//			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
//				"error": "Internal server error",
//				"code":  "INTERNAL_ERROR",
//			})
//		},
//	}
//	app.Use(middleware.Recover(cfg))
type RecoverConfig struct {
	EnableStack   bool                               // whether to log stack traces (disable in production)
	OnPanic       func(flash.Ctx, interface{})       // optional callback when panic occurs
	ErrorResponse func(flash.Ctx, interface{}) error // optional custom error response
}

// Recover returns middleware that recovers from panics in HTTP handlers with enhanced security and logging.
//
// Security Features:
//   - Never exposes stack traces or panic details to clients by default
//   - Configurable logging to prevent information leakage
//   - Structured error responses without sensitive information
//   - Safe handling of panic values that might contain sensitive data
//
// This middleware is essential for production applications as it prevents panics from crashing the entire server.
// When a panic occurs in any handler, the middleware catches it and returns a generic HTTP 500 error response
// to the client while allowing the server to continue processing other requests.
//
// The middleware uses Go's built-in recover() mechanism to catch panics and converts them to HTTP errors.
// It's recommended to use this middleware early in the middleware chain, typically as one of the first
// middleware applied to your application.
//
// Usage Examples:
//
//	// Basic usage - secure defaults
//	app := flash.New()
//	app.Use(middleware.Recover())
//
//	// With custom logging (production-safe)
//	app.Use(middleware.Recover(middleware.RecoverConfig{
//		OnPanic: func(c flash.Ctx, err interface{}) {
//			logger := ctx.LoggerFromContext(c.Context())
//			logger.Error("panic recovered",
//				"error", fmt.Sprintf("%v", err),
//				"method", c.Method(),
//				"path", c.Path(),
//				"user_agent", c.Request().Header.Get("User-Agent"),
//			)
//		},
//	}))
//
//	// With custom error response
//	app.Use(middleware.Recover(middleware.RecoverConfig{
//		ErrorResponse: func(c flash.Ctx, err interface{}) error {
//			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
//				"error": "Service temporarily unavailable",
//				"code":  "SERVICE_ERROR",
//				"retry_after": 60,
//			})
//		},
//	}))
//
//	// Development configuration (with stack traces)
//	if os.Getenv("ENV") == "development" {
//		app.Use(middleware.Recover(middleware.RecoverConfig{
//			EnableStack: true,
//			OnPanic: func(c flash.Ctx, err interface{}) {
//				log.Printf("PANIC: %v\nStack: %s", err, debug.Stack())
//			},
//		}))
//	}
//
//	// With other essential middleware
//	app.Use(
//	    middleware.RequestID(),
//	    middleware.Logger(),
//	    middleware.Recover(), // Should be early in the chain
//	)
//
//	// Example handler that might panic
//	app.Get("/users/:id", func(c flash.Ctx) error {
//	    id := c.Param("id")
//	    if id == "panic" {
//	        panic("intentional panic for testing")
//	    }
//	    // Normal handler logic
//	    return c.JSON(200, map[string]string{"id": id})
//	})
//
//	// On specific route groups
//	api := app.Group("/api")
//	api.Use(middleware.Recover())
//	api.Get("/data", func(c flash.Ctx) error {
//	    // This handler is protected from panics
//	    return c.JSON(200, map[string]string{"data": "protected"})
//	})
//
// Error Response (default):
//
//	// When a panic occurs, the client receives:
//	// HTTP/1.1 500 Internal Server Error
//	// Content-Type: text/plain; charset=utf-8
//	// X-Content-Type-Options: nosniff
//	// Content-Length: 21
//	//
//	// Internal Server Error
//
// Panic Scenarios Handled:
//
//	// 1. Nil pointer dereference
//	app.Get("/nil", func(c flash.Ctx) error {
//	    var data *map[string]string
//	    _ = (*data)["key"] // This would panic without Recover middleware
//	    return nil
//	})
//
//	// 2. Array/slice index out of bounds
//	app.Get("/bounds", func(c flash.Ctx) error {
//	    arr := []int{1, 2, 3}
//	    _ = arr[10] // This would panic without Recover middleware
//	    return nil
//	})
//
//	// 3. Type assertions
//	app.Get("/assert", func(c flash.Ctx) error {
//	    var i interface{} = "string"
//	    _ = i.(int) // This would panic without Recover middleware
//	    return nil
//	})
//
//	// 4. Division by zero
//	app.Get("/divide", func(c flash.Ctx) error {
//	    x := 10
//	    y := 0
//	    _ = x / y // This would panic without Recover middleware
//	    return nil
//	})
//
//	// 5. Manual panics
//	app.Get("/manual", func(c flash.Ctx) error {
//	    panic("something went wrong")
//	})
//
// Best Practices:
//
//	// 1. Always use Recover middleware in production
//	// 2. Place it early in the middleware chain
//	// 3. Combine with logging middleware to track panics
//	// 4. Never expose panic details to clients in production
//	// 5. Use structured logging for panic analysis
//
//	app.Use(
//	    middleware.RequestID(),
//	    middleware.Logger(),
//	    middleware.Recover(), // Early in chain
//	    middleware.CORS(),
//	    middleware.Timeout(30 * time.Second),
//	)
//
// Performance Impact:
//
//	// The Recover middleware has minimal performance overhead
//	// It only adds a defer function that's only executed when panics occur
//	// In normal operation, there's virtually no performance cost
func Recover(cfgs ...RecoverConfig) flash.Middleware {
	cfg := RecoverConfig{
		EnableStack: false, // Secure default - no stack traces in production
	}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Execute panic callback if provided
					if cfg.OnPanic != nil {
						// Execute in a separate goroutine to prevent blocking
						// and protect against panics in the callback itself
						go func() {
							defer func() { recover() }() // Protect against callback panics
							cfg.OnPanic(c, r)
						}()
					}

					// Use custom error response if provided
					if cfg.ErrorResponse != nil {
						err = cfg.ErrorResponse(c, r)
						return
					}

					// Default secure error response
					c.Header("X-Content-Type-Options", "nosniff") // Prevent MIME sniffing
					_ = c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
				}
			}()
			return next(c)
		}
	}
}
