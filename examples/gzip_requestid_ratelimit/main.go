package main

import (
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
)

// main demonstrates gzip, request ID, rate limiting, and buffer middleware in flash.
func main() {
	app := flash.New()

	app.Use(mw.Buffer(mw.BufferConfig{InitialSize: 8 << 10, MaxSize: 256 << 10}))
	app.Use(mw.RequestID(), mw.Gzip(), mw.RateLimit(mw.NewSimpleIPLimiter(5, time.Minute)))

	// GET / returns a large JSON payload with request ID and server time.
	app.GET("/", func(c *flash.Ctx) error {
		id, _ := mw.RequestIDFromContext(c.Context())
		return c.JSON(map[string]any{
			"message":     "hello compressed world",
			"request_id":  id,
			"server_time": time.Now().UTC().Format(time.RFC3339Nano),
			"data":        make([]int, 1000), // larger payload to demonstrate gzip and buffer
		})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
