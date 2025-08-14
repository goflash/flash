package main

import (
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash"
)

// main demonstrates context cancellation and timeout handling in flash.
func main() {
	app := flash.New()
	// GET /work simulates a long-running task, returns early if client disconnects or timeout occurs.
	app.GET("/work", func(c *flash.Ctx) error {
		select {
		case <-time.After(2 * time.Second):
			return c.String(http.StatusOK, "finished")
		case <-c.Context().Done():
			// client gone / timeout hit
			return c.Context().Err()
		}
	})
	log.Fatal(http.ListenAndServe(":8080", app))
}
