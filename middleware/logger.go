package middleware

import (
	"context"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/ctx"
)

// LoggerAttributeKey is the context key for storing custom logger attributes.
// Use this key to store custom attributes that will be included in request logs.
type LoggerAttributeKey struct{}

// LoggerAttributes represents custom attributes to be included in request logs.
// This type allows for efficient storage and retrieval of custom attributes.
type LoggerAttributes struct {
	attrs []any
}

// NewLoggerAttributes creates a new LoggerAttributes instance with the given key-value pairs.
// This function is optimized for zero allocations when possible.
func NewLoggerAttributes(pairs ...any) *LoggerAttributes {
	return &LoggerAttributes{attrs: pairs}
}

// Add appends key-value pairs to the attributes.
// This method is optimized for minimal allocations.
func (la *LoggerAttributes) Add(pairs ...any) {
	la.attrs = append(la.attrs, pairs...)
}

// WithLoggerAttributes adds custom attributes to the context for logging.
// These attributes will be included in the request log when using the Logger middleware.
//
// Usage Examples:
//
//	// Add custom attributes in middleware or handlers
//	attrs := middleware.NewLoggerAttributes("user_id", "123", "operation", "create")
//	ctx := middleware.WithLoggerAttributes(c.Context(), attrs)
//	c.SetRequest(c.Request().WithContext(ctx))
//
//	// Or add attributes directly
//	ctx := middleware.WithLoggerAttributes(c.Context(),
//	    middleware.NewLoggerAttributes("user_id", "123", "operation", "create"))
//	c.SetRequest(c.Request().WithContext(ctx))
func WithLoggerAttributes(ctx context.Context, attrs *LoggerAttributes) context.Context {
	return context.WithValue(ctx, LoggerAttributeKey{}, attrs)
}

// LoggerAttributesFromContext retrieves custom logger attributes from the context.
// Returns nil if no attributes are found.
func LoggerAttributesFromContext(ctx context.Context) *LoggerAttributes {
	if v := ctx.Value(LoggerAttributeKey{}); v != nil {
		if attrs, ok := v.(*LoggerAttributes); ok {
			return attrs
		}
	}
	return nil
}

// LoggerConfig holds configuration options for the Logger middleware.
type LoggerConfig struct {
	// ExcludeFields specifies which standard fields to exclude from logging.
	// Valid values: "method", "path", "route", "status", "duration_ms", "remote", "user_agent", "request_id"
	ExcludeFields []string

	// CustomAttributesFunc is an optional function that can add custom attributes
	// based on the request context. This function is called for each request
	// and should return key-value pairs to be included in the log.
	CustomAttributesFunc func(c flash.Ctx) []any

	// Message is the log message to use. Defaults to "request".
	Message string
}

// LoggerOption is a function that configures the Logger middleware.
type LoggerOption func(*LoggerConfig)

// WithExcludeFields excludes specific standard fields from logging.
//
// Usage Examples:
//
//	// Exclude user agent and remote address for privacy
//	app.Use(middleware.Logger(middleware.WithExcludeFields("user_agent", "remote")))
//
//	// Exclude multiple fields
//	app.Use(middleware.Logger(middleware.WithExcludeFields("user_agent", "remote", "request_id")))
func WithExcludeFields(fields ...string) LoggerOption {
	return func(cfg *LoggerConfig) {
		cfg.ExcludeFields = append(cfg.ExcludeFields, fields...)
	}
}

// WithCustomAttributes adds a function that can provide custom attributes for each request.
//
// Usage Examples:
//
//	// Add user ID from authentication middleware
//	app.Use(middleware.Logger(middleware.WithCustomAttributes(func(c flash.Ctx) []any {
//	    if userID := c.Context().Value("user_id"); userID != nil {
//	        return []any{"user_id", userID}
//	    }
//	    return nil
//	})))
//
//	// Add multiple custom attributes
//	app.Use(middleware.Logger(middleware.WithCustomAttributes(func(c flash.Ctx) []any {
//	    attrs := make([]any, 0, 4)
//	    if userID := c.Context().Value("user_id"); userID != nil {
//	        attrs = append(attrs, "user_id", userID)
//	    }
//	    if tenantID := c.Context().Value("tenant_id"); tenantID != nil {
//	        attrs = append(attrs, "tenant_id", tenantID)
//	    }
//	    return attrs
//	})))
func WithCustomAttributes(fn func(c flash.Ctx) []any) LoggerOption {
	return func(cfg *LoggerConfig) {
		cfg.CustomAttributesFunc = fn
	}
}

// WithMessage sets a custom log message instead of the default "request".
//
// Usage Examples:
//
//	// Use a custom message
//	app.Use(middleware.Logger(middleware.WithMessage("http_request")))
//
//	// Use different messages for different route groups
//	api.Use(middleware.Logger(middleware.WithMessage("api_request")))
//	web.Use(middleware.Logger(middleware.WithMessage("web_request")))
func WithMessage(message string) LoggerOption {
	return func(cfg *LoggerConfig) {
		cfg.Message = message
	}
}

// Logger returns middleware that logs each HTTP request using structured logging (slog).
//
// This middleware automatically captures and logs the following request information:
//   - HTTP method (GET, POST, PUT, DELETE, etc.)
//   - Request path (e.g., "/api/users/123")
//   - Route pattern (e.g., "/api/users/:id")
//   - HTTP status code (200, 404, 500, etc.)
//   - Request duration in milliseconds
//   - Remote client address
//   - User agent string
//   - Request ID (if available via RequestID middleware)
//   - Custom attributes (if provided via context or CustomAttributesFunc)
//
// The logger is retrieved from the request context or application context.
// If no status code is set by the handler, it defaults to 200 (OK).
//
// Usage Examples:
//
//	// Basic usage - add to your app or group
//	app := flash.New()
//	app.Use(middleware.Logger())
//
//	// With custom routes
//	app.Get("/users", func(c flash.Ctx) error {
//	    // Your handler logic here
//	    return c.JSON(200, map[string]string{"message": "success"})
//	})
//
//	// Combined with other middleware
//	app.Use(
//	    middleware.RequestID(),
//	    middleware.Logger(), // Logger will include request_id if available
//	    middleware.Recover(),
//	)
//
//	// On specific route groups
//	api := app.Group("/api")
//	api.Use(middleware.Logger())
//	api.Get("/users", userHandler)
//	api.Post("/users", createUserHandler)
//
//	// With configuration options
//	app.Use(middleware.Logger(
//	    middleware.WithExcludeFields("user_agent", "remote"),
//	    middleware.WithCustomAttributes(func(c flash.Ctx) []any {
//	        if userID := c.Context().Value("user_id"); userID != nil {
//	            return []any{"user_id", userID}
//	        }
//	        return nil
//	    }),
//	    middleware.WithMessage("http_request"),
//	))
//
//	// Add custom attributes in handlers or middleware
//	app.Use(func(next flash.Handler) flash.Handler {
//	    return func(c flash.Ctx) error {
//	        // Add custom attributes to context
//	        attrs := middleware.NewLoggerAttributes("middleware", "auth", "version", "v2")
//	        ctx := middleware.WithLoggerAttributes(c.Context(), attrs)
//	        c.SetRequest(c.Request().WithContext(ctx))
//	        return next(c)
//	    }
//	})
//
//	// In handlers, add dynamic attributes
//	app.Get("/users/:id", func(c flash.Ctx) error {
//	    userID := c.Param("id")
//	    attrs := middleware.NewLoggerAttributes("user_id", userID, "operation", "fetch")
//	    ctx := middleware.WithLoggerAttributes(c.Context(), attrs)
//	    c.SetRequest(c.Request().WithContext(ctx))
//
//	    // Handler logic...
//	    return c.JSON(200, user)
//	})
//
// Example log output:
//
//	{
//	  "level": "INFO",
//	  "msg": "request",
//	  "method": "GET",
//	  "path": "/api/users/123",
//	  "route": "/api/users/:id",
//	  "status": 200,
//	  "duration_ms": 45.2,
//	  "remote": "192.168.1.100:54321",
//	  "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
//	  "request_id": "req_abc123def456",
//	  "user_id": "123",
//	  "operation": "fetch",
//	  "middleware": "auth",
//	  "version": "v2"
//	}
//
// Error handling:
//
//	// The middleware will log the request even if the handler returns an error
//	app.Get("/error", func(c flash.Ctx) error {
//	    return flash.NewError(500, "Internal server error")
//	})
//	// Log output will show status: 500 and the error will be returned to the client
//
// Performance considerations:
//
//	// The middleware has minimal overhead and is safe to use in production
//	// Duration is measured in microseconds and converted to milliseconds for readability
//	// All string operations are efficient and don't allocate unnecessary memory
//	// Custom attributes are efficiently stored and retrieved from context
//	// Excluded fields are handled with minimal performance impact
func Logger(options ...LoggerOption) flash.Middleware {
	// Apply configuration options
	cfg := &LoggerConfig{
		Message: "request",
	}
	for _, option := range options {
		option(cfg)
	}

	// Create exclusion map for O(1) lookups
	excludeMap := make(map[string]bool, len(cfg.ExcludeFields))
	for _, field := range cfg.ExcludeFields {
		excludeMap[field] = true
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			start := time.Now()
			err := next(c)
			dur := time.Since(start)

			status := c.StatusCode()
			if status == 0 {
				status = 200
			}

			ua, remote := "", ""
			if r := c.Request(); r != nil {
				ua = r.UserAgent()
				remote = r.RemoteAddr
			}

			l := ctx.LoggerFromContext(c.Context())

			// Pre-allocate slice with estimated capacity for better performance
			// Standard fields: 8 pairs, custom attributes: variable, request_id: 1 pair
			estimatedCapacity := 18 // 8 standard + 8 custom + 2 request_id
			attrs := make([]any, 0, estimatedCapacity)

			// Add standard fields (only if not excluded)
			if !excludeMap["method"] {
				attrs = append(attrs, "method", c.Method())
			}
			if !excludeMap["path"] {
				attrs = append(attrs, "path", c.Path())
			}
			if !excludeMap["route"] {
				attrs = append(attrs, "route", c.Route())
			}
			if !excludeMap["status"] {
				attrs = append(attrs, "status", status)
			}
			if !excludeMap["duration_ms"] {
				attrs = append(attrs, "duration_ms", float64(dur.Microseconds())/1000.0)
			}
			if !excludeMap["remote"] {
				attrs = append(attrs, "remote", remote)
			}
			if !excludeMap["user_agent"] {
				attrs = append(attrs, "user_agent", ua)
			}

			// Add request_id if available and not excluded
			if !excludeMap["request_id"] {
				if rid, ok := RequestIDFromContext(c.Context()); ok {
					attrs = append(attrs, "request_id", rid)
				}
			}

			// Add custom attributes from context
			if customAttrs := LoggerAttributesFromContext(c.Context()); customAttrs != nil {
				attrs = append(attrs, customAttrs.attrs...)
			}

			// Add custom attributes from function
			if cfg.CustomAttributesFunc != nil {
				if customAttrs := cfg.CustomAttributesFunc(c); len(customAttrs) > 0 {
					attrs = append(attrs, customAttrs...)
				}
			}

			l.Info(cfg.Message, attrs...)
			return err
		}
	}
}
