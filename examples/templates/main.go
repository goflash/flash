package main

import (
	"html/template"
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// tpl is the HTML template for the /hello endpoint.
var tpl = template.Must(template.New("page").Parse(`<html><body><h1>Hello {{.Name}}</h1></body></html>`))

// main starts a goflash web server with a /hello HTML template endpoint.
func main() {
	app := flash.New()
	// GET /hello renders an HTML template with a name parameter.
	app.GET("/hello", func(c *flash.Ctx) error {
		c.Header("Content-Type", "text/html; charset=utf-8")
		return tpl.Execute(c.ResponseWriter(), map[string]string{"Name": c.Query("name")})
	})
	log.Fatal(http.ListenAndServe(":8080", app))
}
