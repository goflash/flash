package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main demonstrates the use of the ANY method to handle all HTTP verbs in flash.
func main() {
	app := flash.New()
	// ANY /ping responds to any HTTP method with the method name and "pong".
	app.ANY("/ping", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, c.Method()+" pong")
	})
	log.Fatal(http.ListenAndServe(":8080", app))
}
