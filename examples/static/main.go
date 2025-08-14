package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
)

// main starts a goflash web server serving static files from multiple directories at /static.
func main() {
	app := flash.New()

	// Serve /static from multiple folders in order (first match wins)
	// Useful for layering build output over public assets, or theme overrides.
	app.StaticDirs("/static", "./public", "./extra")

	// For single directory, use:
	// app.Static("/static", "./public")

	log.Fatal(http.ListenAndServe(":8080", app))
}
