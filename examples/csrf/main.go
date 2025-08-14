package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	mw "github.com/goflash/flash/middleware"
)

// main demonstrates CSRF protection middleware and a simple HTML form in flash.
func main() {
	app := flash.New()
	app.Use(mw.CSRF())

	// GET /form returns a simple HTML form. The CSRF token is set as a cookie.
	app.GET("/form", func(c *flash.Ctx) error {
		// The CSRF token is set as a cookie; client JS or forms should read it and send in header
		return c.String(http.StatusOK, "<form method='POST' action='/submit'><input name='data'><input type='hidden' name='csrf' value='(read from cookie)'><button>Submit</button></form>")
	})

	// POST /submit processes the form. If CSRF token is valid, this will be reached.
	app.POST("/submit", func(c *flash.Ctx) error {
		return c.String(http.StatusOK, "Submitted!")
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
