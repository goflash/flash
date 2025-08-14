package main

import (
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
)

// main demonstrates CORS and per-route timeout middleware in flash.
func main() {
	app := flash.New()

	app.Use(mw.CORS(mw.CORSConfig{
		Origins: []string{"*"},
		Methods: []string{"GET", "POST", "OPTIONS"},
		Headers: []string{"Content-Type"},
		MaxAge:  600,
	}))

	// GET /maybe-slow sleeps for 2s, but is wrapped in a 1s timeout middleware.
	app.GET("/maybe-slow", func(c *flash.Ctx) error {
		time.Sleep(2 * time.Second)
		return c.String(http.StatusOK, "done")
	}, mw.Timeout(mw.TimeoutConfig{Duration: 1 * time.Second}))

	log.Fatal(http.ListenAndServe(":8080", app))
}
