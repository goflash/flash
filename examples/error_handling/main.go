package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// errNotFound is a sentinel error for demonstration.
var errNotFound = errors.New("not found")

// main demonstrates custom error handling and 404 responses in flash.
func main() {
	app := flash.New()

	// Override framework error handling
	app.OnError = func(c *flash.Ctx, err error) {
		switch {
		case errors.Is(err, errNotFound):
			_ = c.String(http.StatusNotFound, "resource not found")
		default:
			_ = c.String(http.StatusInternalServerError, "internal error")
		}
	}

	// Custom 404 handler
	app.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("custom 404"))
	})

	// GET /thing/:id always returns errNotFound
	app.GET("/thing/:id", func(c *flash.Ctx) error { return errNotFound })

	log.Fatal(http.ListenAndServe(":8080", app))
}
