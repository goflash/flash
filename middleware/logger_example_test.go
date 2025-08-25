package middleware

import (
	"context"

	"github.com/goflash/flash/v2"
)

// ExampleLogger demonstrates basic usage of the Logger middleware.
func ExampleLogger() {
	app := flash.New()
	app.Use(Logger())
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleLogger_withExcludeFields demonstrates excluding specific fields from logging.
func ExampleLogger_withExcludeFields() {
	app := flash.New()
	// Exclude user agent and remote address for privacy
	app.Use(Logger(WithExcludeFields("user_agent", "remote")))
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleLogger_withCustomAttributes demonstrates adding custom attributes via function.
func ExampleLogger_withCustomAttributes() {
	app := flash.New()
	app.Use(Logger(WithCustomAttributes(func(c flash.Ctx) []any {
		// Add user ID from authentication context
		if userID := c.Context().Value("user_id"); userID != nil {
			return []any{"user_id", userID}
		}
		return nil
	})))
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleLogger_withCustomMessage demonstrates using a custom log message.
func ExampleLogger_withCustomMessage() {
	app := flash.New()
	app.Use(Logger(WithMessage("http_request")))
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleLogger_withMultipleOptions demonstrates combining multiple configuration options.
func ExampleLogger_withMultipleOptions() {
	app := flash.New()
	app.Use(Logger(
		WithExcludeFields("user_agent", "remote"),
		WithCustomAttributes(func(c flash.Ctx) []any {
			if userID := c.Context().Value("user_id"); userID != nil {
				return []any{"user_id", userID, "operation", "api_call"}
			}
			return []any{"operation", "api_call"}
		}),
		WithMessage("api_request"),
	))
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleWithLoggerAttributes demonstrates adding custom attributes to context.
func ExampleWithLoggerAttributes() {
	app := flash.New()
	app.Use(Logger())

	// Middleware that adds custom attributes
	app.Use(func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Add custom attributes to context
			attrs := NewLoggerAttributes("middleware", "auth", "version", "v2")
			ctx := WithLoggerAttributes(c.Context(), attrs)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	app.GET("/users/:id", func(c flash.Ctx) error {
		// Add dynamic attributes in handler
		userID := c.Param("id")
		attrs := NewLoggerAttributes("user_id", userID, "operation", "fetch")
		ctx := WithLoggerAttributes(c.Context(), attrs)
		c.SetRequest(c.Request().WithContext(ctx))

		return c.JSON(map[string]string{"id": userID})
	})
}

// ExampleNewLoggerAttributes demonstrates creating logger attributes.
func ExampleNewLoggerAttributes() {
	// Create attributes with key-value pairs
	attrs := NewLoggerAttributes("user_id", "123", "operation", "create")

	// Add more attributes
	attrs.Add("tenant_id", "tenant_456", "environment", "production")

	// Use in context
	ctx := context.Background()
	ctx = WithLoggerAttributes(ctx, attrs)

	// The attributes will be included in request logs when using Logger middleware
	_ = ctx
}

// ExampleLogger_differentRouteGroups demonstrates using different logger configurations for different route groups.
func ExampleLogger_differentRouteGroups() {
	app := flash.New()

	// API routes with detailed logging
	api := app.Group("/api")
	api.Use(Logger(
		WithCustomAttributes(func(c flash.Ctx) []any {
			return []any{"service", "api", "version", "v1"}
		}),
		WithMessage("api_request"),
	))
	api.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "users"})
	})

	// Admin routes with minimal logging (exclude sensitive fields)
	admin := app.Group("/admin")
	admin.Use(Logger(
		WithExcludeFields("user_agent", "remote", "request_id"),
		WithCustomAttributes(func(c flash.Ctx) []any {
			return []any{"service", "admin", "access_level", "admin"}
		}),
		WithMessage("admin_request"),
	))
	admin.GET("/stats", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "stats"})
	})

	// Public routes with standard logging
	app.Use(Logger(WithMessage("public_request")))
	app.GET("/", func(c flash.Ctx) error {
		return c.String(200, "Hello World")
	})
}
