package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main demonstrates route grouping and middleware in flash.
func main() {
	app := flash.New()

	// /api group with GET and POST endpoints.
	api := app.Group("/api")
	api.GET("/users/:id", func(c *flash.Ctx) error {
		return c.JSON(map[string]string{"id": c.Param("id")})
	})
	api.POST("/users", func(c *flash.Ctx) error {
		return c.Status(http.StatusCreated).JSON(map[string]any{"ok": true})
	})

	// /admin group with custom middleware and health endpoint.
	admin := app.Group("/admin")
	admin.Use(func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-Admin", "1")
			return next(c)
		}
	})
	admin.GET("/health", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	// /v1 versioned group with version header middleware.
	v1 := app.Group("/v1")
	v1.Use(func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-API-Version", "v1")
			return next(c)
		}
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
