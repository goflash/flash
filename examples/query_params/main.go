package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main starts a goflash web server with a /echo endpoint that returns a query parameter as JSON.
func main() {
	app := flash.New()
	// GET /echo returns a JSON greeting using the "name" query parameter.
	app.GET("/echo", func(c *flash.Ctx) error {
		name := c.Query("name")
		if name == "" {
			name = "world"
		}
		return c.JSON(map[string]string{"hello": name})
	})
	log.Fatal(http.ListenAndServe(":8080", app))
}
