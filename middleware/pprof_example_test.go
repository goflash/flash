package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
)

func ExamplePprof() {
	// Create a new Flash application
	app := flash.New()

	// Mount pprof under the default /debug/pprof prefix
	app.Use(Pprof())

	// Add some routes to your application
	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.GET("/api/users", func(c flash.Ctx) error {
		return c.JSON(map[string]interface{}{
			"users": []string{"user1", "user2", "user3"},
		})
	})

	// Start the server
	fmt.Println("Server starting on :8080")
	fmt.Println("Pprof endpoints available at:")
	fmt.Println("  - http://localhost:8080/debug/pprof/ (index)")
	fmt.Println("  - http://localhost:8080/debug/pprof/profile (CPU profile)")
	fmt.Println("  - http://localhost:8080/debug/pprof/heap (heap profile)")
	fmt.Println("  - http://localhost:8080/debug/pprof/goroutine (goroutine profile)")

	// In a real application, you would start the server like this:
	// app.Listen(":8080")
}

func ExamplePprof_customPrefix() {
	// Create a new Flash application
	app := flash.New()

	// Mount pprof under a custom prefix for security
	app.Use(Pprof(PprofConfig{
		Prefix: "/profiling",
	}))

	// Add your application routes
	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// Start the server
	fmt.Println("Server starting on :8080")
	fmt.Println("Pprof endpoints available at:")
	fmt.Println("  - http://localhost:8080/profiling/ (index)")
	fmt.Println("  - http://localhost:8080/profiling/profile (CPU profile)")
	fmt.Println("  - http://localhost:8080/profiling/heap (heap profile)")

	// In a real application, you would start the server like this:
	// app.Listen(":8080")
}

func ExamplePprof_withAuthentication() {
	// Create a new Flash application
	app := flash.New()

	// Add authentication middleware before pprof
	app.Use(func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Check if the request is for pprof endpoints
			if c.Path() == "/debug/pprof/" || c.Path() == "/debug/pprof/profile" {
				// Require authentication for pprof endpoints
				authHeader := c.Request().Header.Get("Authorization")
				if authHeader != "Bearer secret-token" {
					return c.Status(http.StatusUnauthorized).String(http.StatusUnauthorized, "Unauthorized")
				}
			}
			return next(c)
		}
	})

	// Mount pprof under the default prefix
	app.Use(Pprof())

	// Add your application routes
	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// Start the server
	fmt.Println("Server starting on :8080")
	fmt.Println("Pprof endpoints require Authorization: Bearer secret-token")

	// In a real application, you would start the server like this:
	// app.Listen(":8080")
}

func ExamplePprof_developmentOnly() {
	// Create a new Flash application
	app := flash.New()

	// Only enable pprof in development
	if isDevelopment() {
		app.Use(Pprof())
		fmt.Println("Pprof enabled for development")
	} else {
		fmt.Println("Pprof disabled in production")
	}

	// Add your application routes
	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// Start the server
	fmt.Println("Server starting on :8080")

	// In a real application, you would start the server like this:
	// app.Listen(":8080")
}

func ExamplePprof_withLoadGeneration() {
	// Create a new Flash application
	app := flash.New()

	// Mount pprof under the default prefix
	app.Use(Pprof())

	// Add a route that generates some load for testing
	app.GET("/load", func(c flash.Ctx) error {
		// Simulate some CPU work
		start := time.Now()
		for time.Since(start) < 100*time.Millisecond {
			// Do some work
			_ = 1 + 1
		}
		return c.String(http.StatusOK, "Load generated")
	})

	// Add a route that allocates memory
	app.GET("/memory", func(c flash.Ctx) error {
		// Allocate some memory
		data := make([]byte, 1024*1024) // 1MB
		_ = data
		return c.String(http.StatusOK, "Memory allocated")
	})

	// Start the server
	fmt.Println("Server starting on :8080")
	fmt.Println("Generate load with: curl http://localhost:8080/load")
	fmt.Println("Allocate memory with: curl http://localhost:8080/memory")
	fmt.Println("View profiles at: http://localhost:8080/debug/pprof/")

	// In a real application, you would start the server like this:
	// app.Listen(":8080")
}

// Helper function to determine if we're in development
func isDevelopment() bool {
	// In a real application, this would check environment variables
	// or configuration to determine if we're in development mode
	return true // For this example, always return true
}
