package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

type ctxKey string

const userKey ctxKey = "user-id"

func main() {
	app := flash.New()

	app.GET("/a", func(c *flash.Ctx) error {
		// no value set, Get without default returns nil
		v := c.Get(userKey)
		return c.JSON(map[string]any{"value": v})
	})

	app.GET("/b", func(c *flash.Ctx) error {
		// no value set, Get with default returns fallback
		uid := c.Get(userKey, "guest").(string)
		return c.JSON(map[string]any{"user": uid})
	})

	app.GET("/c", func(c *flash.Ctx) error {
		// set a value, then read it back
		c.Set(userKey, "u123")
		uid := c.Get(userKey, "guest").(string)
		return c.JSON(map[string]any{"user": uid})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
