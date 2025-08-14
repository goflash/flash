package main

import (
	"log"
	"net/http"
	"time"

	"github.com/goflash/flash"
)

// main demonstrates setting and reading cookies in flash.
func main() {
	app := flash.New()

	// GET /set sets a cookie named "sid".
	app.GET("/set", func(c *flash.Ctx) error {
		http.SetCookie(c.ResponseWriter(), &http.Cookie{
			Name:     "sid",
			Value:    "abc123",
			Path:     "/",
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
		})
		return c.String(http.StatusOK, "cookie set")
	})

	// GET /get reads the "sid" cookie if present.
	app.GET("/get", func(c *flash.Ctx) error {
		ck, err := c.Request().Cookie("sid")
		if err != nil {
			return c.String(http.StatusNotFound, "no cookie")
		}
		return c.String(http.StatusOK, "sid="+ck.Value)
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
