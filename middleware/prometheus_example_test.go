package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
)

// ExamplePrometheus demonstrates basic Prometheus middleware usage
func ExamplePrometheus() {
	app := flash.New()

	// Apply Prometheus middleware with default configuration
	app.Use(Prometheus())

	// Add some routes
	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.GET("/users/:id", func(c flash.Ctx) error {
		id := c.Param("id")
		return c.JSON(map[string]string{"id": id, "name": "John Doe"})
	})

	app.POST("/users", func(c flash.Ctx) error {
		return c.Status(http.StatusCreated).JSON(map[string]string{"status": "created"})
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("View metrics at: http://localhost:8080/metrics")

	// Start server
	// http.ListenAndServe(":8080", app)
}

// ExamplePrometheusWithConfig demonstrates custom Prometheus configuration
func ExamplePrometheusWithConfig() {
	app := flash.New()

	// Custom Prometheus configuration
	config := PrometheusConfig{
		Namespace:     "myapp",
		Subsystem:     "api",
		MetricsPath:   "/monitoring/metrics",
		ExposeMetrics: true,
		Labels: map[string]string{
			"environment": "production",
			"version":     "1.0.0",
		},
	}

	app.Use(PrometheusWithConfig(config))

	app.GET("/api/health", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"status": "healthy"})
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("View metrics at: http://localhost:8080/monitoring/metrics")

	// Start server
	// http.ListenAndServe(":8080", app)
}

// ExampleMountPrometheus demonstrates using the MountPrometheus function
func ExampleMountPrometheus() {
	app := flash.New()

	// Mount Prometheus with default configuration
	// This applies the middleware and exposes /metrics endpoint
	MountPrometheus(app)

	app.GET("/", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.GET("/slow", func(c flash.Ctx) error {
		time.Sleep(100 * time.Millisecond)
		return c.String(http.StatusOK, "Slow response")
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("View metrics at: http://localhost:8080/metrics")

	// Start server
	// http.ListenAndServe(":8080", app)
}
