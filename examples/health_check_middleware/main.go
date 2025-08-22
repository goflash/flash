package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/middleware"
)

func main() {
	app := flash.New()

	// Add logging middleware
	app.Use(middleware.Logger())

	// Add health check with default configuration
	middleware.RegisterHealthCheck(app, middleware.HealthCheckConfig{
		Path: "/health",
	})

	// Add health check with custom configuration
	middleware.RegisterHealthCheck(app, middleware.HealthCheckConfig{
		Path:        "/healthz",
		ServiceName: "my-service",
		HealthCheckFunc: func() error {
			// Simulate a health check that might fail
			if time.Now().Second()%10 == 0 { // Fail every 10 seconds
				return errors.New("simulated health check failure")
			}
			return nil
		},
		OnErrorFunc: func(c flash.Ctx, err error) {
			fmt.Printf("Health check failed: %v\n", err)
		},
		OnSuccessFunc: func(c flash.Ctx) {
			fmt.Println("Health check passed")
		},
	})

	// Add convenience method for simple health check
	middleware.RegisterHealthCheck(app, middleware.HealthCheckConfig{Path: "/ready"})

	// Add a regular API route
	app.GET("/api/status", func(c flash.Ctx) error {
		return c.JSON(map[string]interface{}{
			"status":  "running",
			"version": "1.0.0",
		})
	})

	// Add a route that simulates database dependency
	app.GET("/api/data", func(c flash.Ctx) error {
		// Simulate database check
		if time.Now().Second()%15 == 0 { // Fail every 15 seconds
			return c.String(http.StatusInternalServerError, "database connection failed")
		}
		return c.JSON(map[string]interface{}{
			"data": "some data",
		})
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET /health - Basic health check")
	fmt.Println("  GET /healthz - Health check with custom logic")
	fmt.Println("  GET /ready - Simple health check")
	fmt.Println("  GET /api/status - API status")
	fmt.Println("  GET /api/data - API with simulated failures")

	log.Fatal(http.ListenAndServe(":8080", app))
}
