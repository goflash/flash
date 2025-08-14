package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	"github.com/julienschmidt/httprouter"
)

// plainHTTP is a plain net/http handler to be mounted under /plain.
func plainHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("plain http"))
}

// userHTTP is a parameterized net/http handler reading params from context.
func userHTTP(w http.ResponseWriter, r *http.Request) {
	ps := httprouter.ParamsFromContext(r.Context())
	id := ps.ByName("id")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("user=" + id))
}

// main demonstrates mounting net/http handlers and goflash routes together.
func main() {
	app := flash.New()

	// Mount net/http handler under /plain
	app.Mount("/plain", http.HandlerFunc(plainHTTP))

	// Parameterized net/http handler with goflash routing
	app.HandleHTTP(http.MethodGet, "/users/:id", http.HandlerFunc(userHTTP))

	// Also expose goflash routes
	app.GET("/hello", func(c *flash.Ctx) error { return c.String(http.StatusOK, "hi") })

	// app implements http.Handler, so can be used anywhere
	log.Fatal(http.ListenAndServe(":8080", app))
}
