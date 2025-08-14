package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
)

// main demonstrates nested route groups and middleware composition in flash.
func main() {
	app := flash.New()
	app.Use(mw.Recover(), mw.Logger())

	// Top-level API group with X-API header middleware.
	api := app.Group("/api", func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-API", "v1")
			return next(c)
		}
	})

	// Nested version group inheriting prefix and middleware, adds X-Version header.
	v1 := api.Group("/v1", func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-Version", "v1")
			return next(c)
		}
	})

	// Another nested group under /api/v1/admin with X-Admin header middleware.
	admin := v1.Group("/admin", func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-Admin", "1")
			return next(c)
		}
	})

	// GET /api/v1/ping returns "pong"
	v1.GET("/ping", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, "pong")
	})
	// GET /api/v1/admin/stats returns JSON with area info
	admin.GET("/stats", func(c *flash.Ctx) error {
		return c.JSON(map[string]any{"ok": true, "area": "admin"})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
