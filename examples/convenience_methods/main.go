package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/goflash/flash/v2"
)

func main() {
	app := flash.New()

	// Redirect examples
	app.GET("/redirect/temp", func(c flash.Ctx) error {
		return c.RedirectTemporary("/new-location")
	})

	app.GET("/redirect/perm", func(c flash.Ctx) error {
		return c.RedirectPermanent("/permanent-location")
	})

	app.GET("/redirect/custom", func(c flash.Ctx) error {
		return c.Redirect(http.StatusSeeOther, "/custom-redirect")
	})

	// Status response examples
	app.GET("/not-found", func(c flash.Ctx) error {
		return c.NotFound("The requested resource was not found")
	})

	app.GET("/bad-request", func(c flash.Ctx) error {
		return c.BadRequest("Invalid request parameters")
	})

	app.GET("/unauthorized", func(c flash.Ctx) error {
		return c.Unauthorized("Please provide valid credentials")
	})

	app.GET("/forbidden", func(c flash.Ctx) error {
		return c.Forbidden("You don't have permission to access this resource")
	})

	app.GET("/server-error", func(c flash.Ctx) error {
		return c.InternalServerError("Something went wrong on our end")
	})

	app.GET("/no-content", func(c flash.Ctx) error {
		return c.NoContent()
	})

	// Cookie examples
	app.GET("/set-cookie", func(c flash.Ctx) error {
		cookie := &http.Cookie{
			Name:     "session",
			Value:    "abc123",
			Path:     "/",
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Secure:   false, // Set to true in production with HTTPS
		}
		c.SetCookie(cookie)
		return c.JSON(map[string]string{"message": "Cookie set successfully"})
	})

	app.GET("/get-cookie", func(c flash.Ctx) error {
		cookie, err := c.GetCookie("session")
		if err != nil {
			return c.NotFound("Cookie not found")
		}
		return c.JSON(map[string]string{"session": cookie.Value})
	})

	app.GET("/clear-cookie", func(c flash.Ctx) error {
		c.ClearCookie("session")
		return c.JSON(map[string]string{"message": "Cookie cleared successfully"})
	})

	// File serving examples
	app.GET("/file/:filename", func(c flash.Ctx) error {
		filename := c.Param("filename")
		return c.File(filename)
	})

	// Stream examples
	app.GET("/stream/text", func(c flash.Ctx) error {
		content := "This is a streamed text response\nLine 2\nLine 3"
		reader := strings.NewReader(content)
		return c.Stream(http.StatusOK, "text/plain", reader)
	})

	app.GET("/stream/json", func(c flash.Ctx) error {
		data := map[string]interface{}{
			"message":   "Streamed JSON response",
			"timestamp": time.Now().Unix(),
			"items":     []string{"item1", "item2", "item3"},
		}
		return c.StreamJSON(http.StatusOK, data)
	})

	// Error handling with convenience methods
	app.GET("/api/users/:id", func(c flash.Ctx) error {
		userID := c.Param("id")

		// Simulate different error scenarios
		switch userID {
		case "0":
			return c.BadRequest("Invalid user ID")
		case "999":
			return c.NotFound("User not found")
		case "403":
			return c.Forbidden("Access denied to this user")
		case "500":
			return c.InternalServerError("Database error")
		default:
			return c.JSON(map[string]interface{}{
				"id":    userID,
				"name":  "John Doe",
				"email": "john@example.com",
			})
		}
	})

	// Authentication example
	app.GET("/protected", func(c flash.Ctx) error {
		// Check for authentication token
		token := c.Query("token")
		if token == "" {
			return c.Unauthorized("Authentication token required")
		}

		if token != "valid-token" {
			return c.Forbidden("Invalid authentication token")
		}

		return c.JSON(map[string]string{"message": "Access granted to protected resource"})
	})

	// API response examples
	app.GET("/api/status", func(c flash.Ctx) error {
		status := c.Query("status")

		switch status {
		case "success":
			return c.JSON(map[string]string{"status": "success", "message": "Operation completed"})
		case "error":
			return c.BadRequest("Operation failed")
		case "empty":
			return c.NoContent()
		default:
			return c.JSON(map[string]string{"status": "unknown"})
		}
	})

	// Chaining example
	app.GET("/chained", func(c flash.Ctx) error {
		// Set custom headers and status, then send response
		c.Status(http.StatusCreated).Header("X-Custom", "chained-example")
		return c.JSON(map[string]string{"message": "Response with custom status and headers"})
	})

	log.Println("Server starting on :8080")
	log.Println("Try these endpoints:")
	log.Println("  GET /redirect/temp - Temporary redirect")
	log.Println("  GET /redirect/perm - Permanent redirect")
	log.Println("  GET /not-found - 404 with custom message")
	log.Println("  GET /bad-request - 400 with custom message")
	log.Println("  GET /unauthorized - 401 with custom message")
	log.Println("  GET /forbidden - 403 with custom message")
	log.Println("  GET /server-error - 500 with custom message")
	log.Println("  GET /no-content - 204 No Content")
	log.Println("  GET /set-cookie - Set a session cookie")
	log.Println("  GET /get-cookie - Get the session cookie")
	log.Println("  GET /clear-cookie - Clear the session cookie")
	log.Println("  GET /stream/text - Stream text content")
	log.Println("  GET /stream/json - Stream JSON content")
	log.Println("  GET /api/users/123 - Get user (success)")
	log.Println("  GET /api/users/0 - Get user (bad request)")
	log.Println("  GET /api/users/999 - Get user (not found)")
	log.Println("  GET /protected?token=valid-token - Protected resource")
	log.Println("  GET /protected - Protected resource (unauthorized)")
	log.Println("  GET /api/status?status=success - API status response")
	log.Println("  GET /chained - Chained method calls")

	log.Fatal(http.ListenAndServe(":8080", app))
}
