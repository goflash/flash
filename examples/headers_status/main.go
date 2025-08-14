package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main starts a goflash web server with a /status endpoint that sets a header and status code.
func main() {
	app := flash.New()
	// GET /status sets a custom header and returns 202 Accepted.
	app.GET("/status", func(c *flash.Ctx) error {
		c.Header("X-From", "goflash")
		return c.Status(http.StatusAccepted).String(http.StatusAccepted, "accepted")
	})
	log.Fatal(http.ListenAndServe(":8080", app))
}
