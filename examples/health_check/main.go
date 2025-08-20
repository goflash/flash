package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash/v2"
)

// main demonstrates the health check functionality in GoFlash.
func main() {
	app := flash.New()

	// Enable health check at /health
	app.EnableHealthCheck("/health")

	// Add a custom health check function
	app.SetHealthCheck(func() error {
		// Simulate checking database connection
		// In a real application, you would check:
		// - Database connectivity
		// - External service dependencies
		// - Disk space
		// - Memory usage
		// - etc.

		// For demo purposes, let's simulate a healthy service
		// Uncomment the line below to simulate an unhealthy service
		// return errors.New("database connection failed")

		return nil // Service is healthy
	})

	// Add some regular routes
	app.GET("/", func(c flash.Ctx) error {
		return c.JSON(map[string]string{
			"message": "Welcome to GoFlash with Health Check!",
			"health":  "Check /health for service status",
		})
	})

	app.GET("/api/users", func(c flash.Ctx) error {
		return c.JSON(map[string]interface{}{
			"users": []map[string]string{
				{"id": "1", "name": "Alice"},
				{"id": "2", "name": "Bob"},
			},
		})
	})

	// Start the server
	log.Println("Server starting on :8080")
	log.Println("Health check available at: http://localhost:8080/health")
	log.Println("Main app available at: http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", app))
}
