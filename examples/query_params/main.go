package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main starts a goflash web server with endpoints that demonstrate type-safe query parameter helpers.
func main() {
	app := flash.New()

	// GET /echo returns a JSON greeting using the "name" query parameter (string-based).
	app.GET("/echo", func(c *flash.Ctx) error {
		name := c.Query("name")
		if name == "" {
			name = "world"
		}
		return c.JSON(map[string]string{"hello": name})
	})

	// GET /calculator demonstrates type-safe query parameter helpers.
	app.GET("/calculator", func(c *flash.Ctx) error {
		// Type-safe query parameter parsing with defaults
		a := c.QueryInt("a", 0)
		b := c.QueryInt("b", 0)
		operation := c.Query("op")

		var result int
		switch operation {
		case "add":
			result = a + b
		case "subtract":
			result = a - b
		case "multiply":
			result = a * b
		case "divide":
			if b != 0 {
				result = a / b
			} else {
				return c.JSON(map[string]string{"error": "division by zero"})
			}
		default:
			return c.JSON(map[string]string{"error": "invalid operation"})
		}

		return c.JSON(map[string]interface{}{
			"operation": operation,
			"a":         a,
			"b":         b,
			"result":    result,
		})
	})

	// GET /settings demonstrates boolean and numeric query parameters.
	app.GET("/settings", func(c *flash.Ctx) error {
		// Boolean query parameters with various formats
		debug := c.QueryBool("debug", false)
		verbose := c.QueryBool("verbose", false)

		// Numeric parameters with defaults
		timeout := c.QueryInt("timeout", 30)
		limit := c.QueryInt64("limit", 100)
		ratio := c.QueryFloat64("ratio", 1.0)

		return c.JSON(map[string]interface{}{
			"debug":   debug,
			"verbose": verbose,
			"timeout": timeout,
			"limit":   limit,
			"ratio":   ratio,
		})
	})

	// GET /users/:id demonstrates type-safe path parameters.
	app.GET("/users/:id", func(c *flash.Ctx) error {
		// Type-safe path parameter parsing
		userID := c.ParamInt("id", 0)
		if userID == 0 {
			return c.JSON(map[string]string{"error": "invalid user ID"})
		}

		// Query parameters for pagination
		page := c.QueryInt("page", 1)
		perPage := c.QueryInt("per_page", 10)
		includeDetails := c.QueryBool("details", false)

		return c.JSON(map[string]interface{}{
			"user_id":         userID,
			"page":            page,
			"per_page":        perPage,
			"include_details": includeDetails,
			"message":         "User details would be fetched here",
		})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
