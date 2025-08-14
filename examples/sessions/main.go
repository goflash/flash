package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
)

// main starts a goflash web server demonstrating session usage and API grouping.
func main() {
	app := flash.New()

	// Use sessions with defaults (cookie-based, in-memory store)
	app.Use(mw.Sessions(mw.SessionConfig{}))

	// GET /set sets a session value.
	app.GET("/set", func(c *flash.Ctx) error {
		sess := mw.SessionFromCtx(c)
		sess.Set("count", 1)
		return c.String(http.StatusOK, "set")
	})

	// GET /get reads a session value if present.
	app.GET("/get", func(c *flash.Ctx) error {
		sess := mw.SessionFromCtx(c)
		if v, ok := sess.Get("count"); ok {
			return c.String(http.StatusOK, fmt.Sprintf("count=%v", v))
		}
		return c.String(http.StatusNotFound, "no session value")
	})

	// Header-based session id example
	api := app.Group("/api", mw.Sessions(mw.SessionConfig{HeaderName: "X-Session-ID"}))
	// GET /api/me sets and returns a user session value as JSON.
	api.GET("/me", func(c *flash.Ctx) error {
		sess := mw.SessionFromCtx(c)
		sess.Set("user", "u1")
		return c.JSON(map[string]any{"user": "u1"})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
