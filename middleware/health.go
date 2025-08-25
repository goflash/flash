package middleware

import (
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/ctx"
	"github.com/goflash/flash/v2/security"
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

	// Sanitize the path
		// Log a warning if path sanitization fails, then use default
		// Using standard log package for simplicity
		// You may replace this with your preferred logging mechanism
		// (e.g., a package-level logger or app logger if available)
		// import "log" at the top if not already imported
		// log.Printf("WARNING: Invalid health check path '%s', falling back to '/health'", cfg.Path)
		// Since imports can't be changed, use the standard log package if available, else comment
		// For this snippet, let's assume log is available
		// If not, this can be uncommented when log is imported
		// log.Printf("WARNING: Invalid health check path '%s', falling back to '/health'", cfg.Path)
		// If log is not available, you can use fmt.Println as a fallback
		// fmt.Printf("WARNING: Invalid health check path '%s', falling back to '/health'\n", cfg.Path)
		// For now, use fmt.Println
		println("WARNING: Invalid health check path '" + cfg.Path + "', falling back to '/health'")
		sanitizedPath = "/health"
	}

	// Register the health check route
	app.GET(sanitizedPath, healthCheckHandler(cfg))
}

// HealthCheckWithPath is a convenience function that creates a health check configuration
// with just a path and optional health check function.
func HealthCheckWithPath(path string, fn ...HealthCheckFunc) HealthCheckConfig {
	cfg := HealthCheckConfig{Path: path, ServiceName: "goflash"}
	if len(fn) > 0 {
		cfg.HealthCheckFunc = fn[0]
	}
	return cfg
}
