package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goflash/flash"
)

// main demonstrates graceful shutdown of a goflash web server on interrupt signal.
func main() {
	app := flash.New()
	// GET / returns a simple OK response.
	app.GET("/", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	h := &http.Server{Addr: ":8080", Handler: app}

	go func() {
		log.Println("listening on :8080")
		if err := h.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.Shutdown(ctx)
	log.Println("server stopped")
}
