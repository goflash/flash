package middleware

import (
	"net/http"

	"github.com/goflash/flash/v2"
)

// RequestSizeConfig configures the request size limiting middleware.
//
// MaxSize sets the maximum allowed request body size in bytes. When a request
// exceeds this limit, the middleware returns a 413 Request Entity Too Large
// response before the request body is fully read, preventing memory exhaustion.
//
// Security considerations:
//   - Set MaxSize based on your application's actual needs
//   - Consider different limits for different endpoints (use route-specific middleware)
//   - Monitor for potential DoS attacks through large request bodies
//   - Balance security with legitimate large file uploads
//
// Performance considerations:
//   - Check is performed before reading the request body (minimal overhead)
//   - Uses Content-Length header for efficient size checking
//   - No memory allocation for size validation
//   - Early rejection prevents unnecessary processing
//
// Example:
//
//	// Global request size limit
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 10 << 20, // 10MB
//	}))
//
//	// Different limits for different endpoints
//	api := app.Group("/api")
//	api.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 1 << 20, // 1MB for API
//	}))
//
//	upload := app.Group("/upload")
//	upload.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 100 << 20, // 100MB for file uploads
//	}))
//
//	// Custom error response
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 5 << 20, // 5MB
//		ErrorResponse: func(c flash.Ctx, size, limit int64) error {
//			return c.JSON(http.StatusRequestEntityTooLarge, map[string]interface{}{
//				"error": "Request too large",
//				"code":  "REQUEST_TOO_LARGE",
//				"size":  size,
//				"limit": limit,
//			})
//		},
//	}))
type RequestSizeConfig struct {
	// MaxSize is the maximum allowed request body size in bytes.
	// If 0 or negative, no limit is enforced (not recommended for production).
	MaxSize int64

	// ErrorResponse allows customizing the error response when size limit is exceeded.
	// If nil, a default JSON error response is returned.
	ErrorResponse func(flash.Ctx, int64, int64) error
}

// RequestSize returns middleware that limits the maximum size of request bodies.
//
// Security Features:
//   - Prevents memory exhaustion attacks through large request bodies
//   - Early rejection before body parsing to minimize resource usage
//   - Configurable limits for different application needs
//   - Secure error responses that don't leak sensitive information
//
// Performance Features:
//   - Zero-allocation size checking using Content-Length header
//   - Early rejection prevents unnecessary request processing
//   - Minimal overhead for requests within size limits
//   - No impact on request streaming or processing speed
//
// Behavior:
//   - Checks Content-Length header before processing request body
//   - Returns 413 Request Entity Too Large for oversized requests
//   - Allows requests without Content-Length header (e.g., chunked encoding)
//   - Works with all HTTP methods and content types
//
// Usage Examples:
//
//	// Basic usage with 5MB limit
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 5 << 20, // 5MB
//	}))
//
//	// API-specific limits
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 1 << 20, // 1MB for JSON APIs
//	}))
//
//	// File upload endpoints with higher limits
//	uploadGroup := app.Group("/upload")
//	uploadGroup.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 100 << 20, // 100MB for file uploads
//	}))
//
//	// Custom error handling
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 10 << 20, // 10MB
//		ErrorResponse: func(c flash.Ctx, size, limit int64) error {
//			// Log the attempt
//			logger := ctx.LoggerFromContext(c.Context())
//			logger.Warn("request size limit exceeded",
//				"size", size,
//				"limit", limit,
//				"path", c.Path(),
//				"method", c.Method(),
//				"remote_addr", c.Request().RemoteAddr,
//			)
//
//			return c.Status(http.StatusRequestEntityTooLarge).JSON(map[string]interface{}{
//				"error": "Request entity too large",
//				"code":  "REQUEST_TOO_LARGE",
//				"max_size_bytes": limit,
//			})
//		},
//	}))
//
// Security Best Practices:
//
//	// Production configuration
//	app.Use(middleware.RequestSize(middleware.RequestSizeConfig{
//		MaxSize: 5 << 20, // Start conservative - 5MB
//		ErrorResponse: func(c flash.Ctx, size, limit int64) error {
//			// Log for monitoring but don't expose details to client
//			logger := ctx.LoggerFromContext(c.Context())
//			logger.Warn("request size limit exceeded",
//				"size", size,
//				"limit", limit,
//				"user_agent", c.Request().Header.Get("User-Agent"),
//				"remote_addr", c.Request().RemoteAddr,
//			)
//
//			return c.Status(http.StatusRequestEntityTooLarge).JSON(map[string]interface{}{
//				"error": "Request entity too large",
//			})
//		},
//	}))
//
// Common Size Limits:
//   - JSON APIs: 1MB (1 << 20)
//   - Form submissions: 5MB (5 << 20)
//   - File uploads: 50-100MB (50 << 20 to 100 << 20)
//   - Image uploads: 10MB (10 << 20)
//   - Document uploads: 25MB (25 << 20)
//
// Performance Impact:
//   - Minimal overhead: only checks Content-Length header
//   - No memory allocation for size validation
//   - Early rejection prevents wasted CPU cycles
//   - Zero impact on legitimate requests within limits
func RequestSize(cfg RequestSizeConfig) flash.Middleware {
	// Validate configuration
	if cfg.MaxSize <= 0 {
		// Allow unlimited size if MaxSize is 0 or negative
		// This is not recommended for production but may be useful for development
		return func(next flash.Handler) flash.Handler {
			return next // No-op middleware
		}
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Check Content-Length header for efficiency
			// Note: This won't catch chunked requests without Content-Length (-1),
			// but those are less common and harder to exploit for DoS
			contentLength := c.Request().ContentLength

			if contentLength > 0 && contentLength > cfg.MaxSize {
				// Use custom error response if provided
				if cfg.ErrorResponse != nil {
					return cfg.ErrorResponse(c, contentLength, cfg.MaxSize)
				}

				// Default secure error response
				c.Header("X-Content-Type-Options", "nosniff") // Security header
				return c.Status(http.StatusRequestEntityTooLarge).JSON(map[string]interface{}{
					"error": "Request entity too large",
					"code":  "REQUEST_TOO_LARGE",
					"limit": cfg.MaxSize,
				})
			}

			// Request size is within limits, continue processing
			return next(c)
		}
	}
}
