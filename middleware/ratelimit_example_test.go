package middleware

import (
	"time"

	"github.com/goflash/flash/v2"
)

// ExampleRateLimit demonstrates basic usage of the RateLimit middleware.
func ExampleRateLimit() {
	app := flash.New()
	strategy := NewTokenBucketStrategy(100, time.Minute)
	app.Use(RateLimit(WithStrategy(strategy)))
	app.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "success"})
	})
}

// ExampleRateLimit_withTokenBucket demonstrates token bucket strategy.
func ExampleRateLimit_withTokenBucket() {
	app := flash.New()

	// Token bucket allows bursts up to capacity, then refills over time
	strategy := NewTokenBucketStrategy(50, time.Minute) // 50 requests per minute with burst support
	app.Use(RateLimit(WithStrategy(strategy)))

	app.GET("/api", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "api response"})
	})
}

// ExampleRateLimit_withFixedWindow demonstrates fixed window strategy.
func ExampleRateLimit_withFixedWindow() {
	app := flash.New()

	// Fixed window resets counter at fixed intervals (allows bursts at boundaries)
	strategy := NewFixedWindowStrategy(100, time.Hour) // 100 requests per hour window
	app.Use(RateLimit(WithStrategy(strategy)))

	app.GET("/public", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "public response"})
	})
}

// ExampleRateLimit_withSlidingWindow demonstrates sliding window strategy.
func ExampleRateLimit_withSlidingWindow() {
	app := flash.New()

	// Sliding window provides smooth rate limiting without burst issues
	strategy := NewSlidingWindowStrategy(10, time.Minute) // 10 requests per 1-minute sliding window
	app.Use(RateLimit(WithStrategy(strategy)))

	app.GET("/sensitive", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "sensitive response"})
	})
}

// ExampleRateLimit_withLeakyBucket demonstrates leaky bucket strategy.
func ExampleRateLimit_withLeakyBucket() {
	app := flash.New()

	// Leaky bucket processes requests at fixed rate, queues excess
	strategy := NewLeakyBucketStrategy(5.0, 10) // 5 requests per second, capacity for 10 queued
	app.Use(RateLimit(WithStrategy(strategy)))

	app.GET("/streaming", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "streaming response"})
	})
}

// ExampleRateLimit_withAdaptive demonstrates adaptive strategy.
func ExampleRateLimit_withAdaptive() {
	app := flash.New()

	// Adaptive strategy adjusts rate based on client behavior
	strategy := NewAdaptiveStrategy(10.0, 1.0, 100.0, time.Minute) // 10-100 req/sec range
	app.Use(RateLimit(WithStrategy(strategy)))

	app.GET("/adaptive", func(c flash.Ctx) error {
		// In your application logic, provide feedback to the strategy
		// strategy.UpdateRate(clientKey, true)  // Good behavior
		// strategy.UpdateRate(clientKey, false) // Bad behavior
		return c.JSON(map[string]string{"message": "adaptive response"})
	})
}

// ExampleRateLimit_withCustomKeyFunc demonstrates custom key extraction.
func ExampleRateLimit_withCustomKeyFunc() {
	app := flash.New()
	strategy := NewTokenBucketStrategy(10, time.Minute)

	// Rate limit by user ID instead of IP
	app.Use(RateLimit(
		WithStrategy(strategy),
		WithKeyFunc(func(c flash.Ctx) string {
			if userID := c.Context().Value("user_id"); userID != nil {
				return userID.(string)
			}
			return "anonymous"
		}),
	))

	app.GET("/user", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "user response"})
	})
}

// ExampleRateLimit_withCustomErrorResponse demonstrates custom error responses.
func ExampleRateLimit_withCustomErrorResponse() {
	app := flash.New()
	strategy := NewTokenBucketStrategy(5, time.Minute)

	// Custom error response with JSON format
	app.Use(RateLimit(
		WithStrategy(strategy),
		WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
			return c.JSON(map[string]interface{}{
				"error":       "Rate limit exceeded",
				"retry_after": int(retryAfter.Seconds()),
				"limit_type":  "token_bucket",
			})
		}),
	))

	app.GET("/api", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "api response"})
	})
}

// ExampleRateLimit_withSkipFunc demonstrates skipping rate limiting for certain requests.
func ExampleRateLimit_withSkipFunc() {
	app := flash.New()
	strategy := NewTokenBucketStrategy(10, time.Minute)

	// Skip rate limiting for admin users or health checks
	app.Use(RateLimit(
		WithStrategy(strategy),
		WithSkipFunc(func(c flash.Ctx) bool {
			// Skip for admin users
			if c.Context().Value("admin") == true {
				return true
			}
			// Skip for health checks
			if c.Path() == "/health" {
				return true
			}
			return false
		}),
	))

	app.GET("/api", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "api response"})
	})

	app.GET("/health", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"status": "healthy"})
	})
}

// ExampleRateLimit_differentStrategiesForDifferentRoutes demonstrates using different strategies for different route groups.
func ExampleRateLimit_differentStrategiesForDifferentRoutes() {
	app := flash.New()

	// Public routes with generous limits
	publicStrategy := NewTokenBucketStrategy(1000, time.Hour)
	app.Use(RateLimit(WithStrategy(publicStrategy)))
	app.GET("/", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "public"})
	})

	// API routes with moderate limits
	api := app.Group("/api")
	apiStrategy := NewSlidingWindowStrategy(100, time.Minute)
	api.Use(RateLimit(WithStrategy(apiStrategy)))
	api.GET("/users", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "api users"})
	})

	// Admin routes with strict limits
	admin := app.Group("/admin")
	adminStrategy := NewFixedWindowStrategy(10, time.Hour)
	admin.Use(RateLimit(WithStrategy(adminStrategy)))
	admin.GET("/stats", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "admin stats"})
	})

	// Authentication routes with very strict limits
	auth := app.Group("/auth")
	authStrategy := NewLeakyBucketStrategy(1.0, 5) // 1 request per second
	auth.Use(RateLimit(WithStrategy(authStrategy)))
	auth.POST("/login", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "login"})
	})
}

// ExampleRateLimit_combinedOptions demonstrates combining multiple configuration options.
func ExampleRateLimit_combinedOptions() {
	app := flash.New()
	strategy := NewTokenBucketStrategy(50, time.Minute)

	// Combine multiple options for sophisticated rate limiting
	app.Use(RateLimit(
		WithStrategy(strategy),
		WithKeyFunc(func(c flash.Ctx) string {
			// Rate limit by user ID if available, otherwise by IP
			if userID := c.Context().Value("user_id"); userID != nil {
				return "user:" + userID.(string)
			}
			return "ip:" + c.Request().RemoteAddr
		}),
		WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
			return c.JSON(map[string]interface{}{
				"error":       "Too many requests",
				"retry_after": int(retryAfter.Seconds()),
				"limit":       50,
				"window":      "1 minute",
			})
		}),
		WithSkipFunc(func(c flash.Ctx) bool {
			// Skip for internal health checks
			return c.Path() == "/health" || c.Context().Value("internal") == true
		}),
	))

	app.GET("/api", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "api response"})
	})
}

// ExampleRateLimit_defaultStrategy demonstrates using the default strategy.
func ExampleRateLimit_defaultStrategy() {
	app := flash.New()

	// Use RateLimit without specifying a strategy (uses default token bucket)
	app.Use(RateLimit()) // Default: 100 requests per minute per IP

	app.GET("/default", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"message": "default response"})
	})
}

// ExampleNewTokenBucketStrategy demonstrates creating a token bucket strategy.
func ExampleNewTokenBucketStrategy() {
	// 100 requests per minute with burst support
	strategy := NewTokenBucketStrategy(100, time.Minute)

	// 10 requests per second (very restrictive)
	strictStrategy := NewTokenBucketStrategy(10, time.Second)

	// 1000 requests per hour
	hourlyStrategy := NewTokenBucketStrategy(1000, time.Hour)

	// Use in middleware
	app := flash.New()
	app.Use(RateLimit(WithStrategy(strategy)))

	_ = strictStrategy
	_ = hourlyStrategy
}

// ExampleNewFixedWindowStrategy demonstrates creating a fixed window strategy.
func ExampleNewFixedWindowStrategy() {
	// 100 requests per hour window
	strategy := NewFixedWindowStrategy(100, time.Hour)

	// 10 requests per day window
	dailyStrategy := NewFixedWindowStrategy(10, 24*time.Hour)

	// Use in middleware
	app := flash.New()
	app.Use(RateLimit(WithStrategy(strategy)))

	_ = dailyStrategy
}

// ExampleNewSlidingWindowStrategy demonstrates creating a sliding window strategy.
func ExampleNewSlidingWindowStrategy() {
	// 50 requests per 5-minute sliding window
	strategy := NewSlidingWindowStrategy(50, 5*time.Minute)

	// 1000 requests per hour sliding window
	hourlyStrategy := NewSlidingWindowStrategy(1000, time.Hour)

	// Use in middleware
	app := flash.New()
	app.Use(RateLimit(WithStrategy(strategy)))

	_ = hourlyStrategy
}

// ExampleNewLeakyBucketStrategy demonstrates creating a leaky bucket strategy.
func ExampleNewLeakyBucketStrategy() {
	// 5 requests per second with capacity for 20 queued requests
	strategy := NewLeakyBucketStrategy(5.0, 20)

	// 0.1 requests per second (1 request every 10 seconds)
	slowStrategy := NewLeakyBucketStrategy(0.1, 5)

	// Use in middleware
	app := flash.New()
	app.Use(RateLimit(WithStrategy(strategy)))

	_ = slowStrategy
}

// ExampleNewAdaptiveStrategy demonstrates creating an adaptive strategy.
func ExampleNewAdaptiveStrategy() {
	// Adaptive rate limiting with 10-100 requests per second range
	strategy := NewAdaptiveStrategy(50.0, 10.0, 100.0, time.Minute)

	// Very restrictive adaptive limiting
	strictStrategy := NewAdaptiveStrategy(1.0, 0.1, 10.0, time.Minute)

	// Use in middleware
	app := flash.New()
	app.Use(RateLimit(WithStrategy(strategy)))

	_ = strictStrategy
}
