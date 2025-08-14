package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main starts a basic goflash web server with a single /hello route.
func main() {
	app := flash.New()

	// GET /hello returns a plain text greeting.
	app.GET("/hello", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, "Hello, world!")
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
