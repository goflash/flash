package middleware

import (
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/ctx"
)

// HealthCheckFunc is a function that performs health checks and returns an error if unhealthy.
// If the function returns nil, the service is considered healthy.
type HealthCheckFunc func() error

// OnErrorFunc is called when the health check fails.
// It receives the context and the error from the health check function.
type OnErrorFunc func(flash.Ctx, error)

// OnSuccessFunc is called when the health check succeeds.
// It receives the context and can be used for logging or metrics.
type OnSuccessFunc func(flash.Ctx)

// HealthCheckConfig configures the health check middleware.
type HealthCheckConfig struct {
	// Path is the endpoint path for the health check (e.g., "/health", "/healthz").
	// Defaults to "/health" if not provided.
	Path string
	// HealthCheckFunc is the function that performs the actual health check.
	// If nil, a default health check that always returns healthy is used.
	HealthCheckFunc HealthCheckFunc
	// OnErrorFunc is called when the health check fails.
	// If nil, errors are logged using the app logger.
	OnErrorFunc OnErrorFunc
	// OnSuccessFunc is called when the health check succeeds.
	// If nil, no action is taken.
	OnSuccessFunc OnSuccessFunc
	// ServiceName is the name of the service to include in the response.
	// Defaults to "goflash" if not provided.
	ServiceName string
}

// healthCheckHandler creates a health check handler function
func healthCheckHandler(cfg HealthCheckConfig) flash.Handler {
	return func(c flash.Ctx) error {
		// Perform health check
		var err error
		if cfg.HealthCheckFunc != nil {
			err = cfg.HealthCheckFunc()
		}

		// Determine response
		status := "healthy"
		httpStatus := http.StatusOK

		if err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable

			// Call error handler if provided
			if cfg.OnErrorFunc != nil {
				cfg.OnErrorFunc(c, err)
			} else {
				// Default error logging
				l := ctx.LoggerFromContext(c.Context())
				l.Error("health check failed", "error", err)
			}
		} else {
			// Call success handler if provided
			if cfg.OnSuccessFunc != nil {
				cfg.OnSuccessFunc(c)
			}
		}

		// Build response
		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"service":   cfg.ServiceName,
		}

		// Add error details if unhealthy
		if err != nil {
			response["error"] = err.Error()
		}

		return c.Status(httpStatus).JSON(response)
	}
}

// HealthCheck returns middleware that adds a health check endpoint.
// The middleware registers a GET route at the specified path that returns
// a JSON response indicating the health status of the service.
// Note: This middleware only works if a route is already registered at the health check path.
// For automatic route registration, use RegisterHealthCheck instead.
func HealthCheck(cfg HealthCheckConfig) flash.Middleware {
	// Set defaults
	if cfg.Path == "" {
		cfg.Path = "/health"
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "goflash"
	}

	// Create the health check handler
	handler := healthCheckHandler(cfg)

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Only handle requests to the health check path
			if c.Path() == cfg.Path {
				return handler(c)
			}
			return next(c)
		}
	}
}

// RegisterHealthCheck registers a health check endpoint on the given app.
// This is the recommended way to add health checks as it automatically registers the route.
func RegisterHealthCheck(app flash.App, cfg HealthCheckConfig) {
	// Set defaults
	if cfg.Path == "" {
		cfg.Path = "/health"
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "goflash"
	}

	// Register the health check route
	app.GET(cfg.Path, healthCheckHandler(cfg))
}

// HealthCheckWithPath is a convenience function that creates a health check middleware
// with just a path and optional health check function.
func HealthCheckWithPath(path string, fn ...HealthCheckFunc) flash.Middleware {
	cfg := HealthCheckConfig{Path: path}
	if len(fn) > 0 {
		cfg.HealthCheckFunc = fn[0]
	}
	return HealthCheck(cfg)
}
