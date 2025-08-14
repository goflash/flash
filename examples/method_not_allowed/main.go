package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main demonstrates a custom 405 Method Not Allowed handler in flash.
func main() {
	app := flash.New()
	// Custom 405 handler for unsupported methods.
	app.MethodNA = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("custom 405"))
	})

	// GET /thing returns a simple OK response.
	app.GET("/thing", func(c *flash.Ctx) error { return c.String(http.StatusOK, "GET ok") })
	log.Fatal(http.ListenAndServe(":8080", app))
}
