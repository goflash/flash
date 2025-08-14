package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

func main() {
	app := flash.New()

	// Add a simple middleware
	app.Use(func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			c.Header("X-Middleware", "1")
			return next(c)
		}
	})

	app.GET("/", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, "middleware ok")
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
