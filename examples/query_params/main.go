package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash/v2"
)

func main() {
	app := flash.New()

	// Example: GET /users/:id?active=true&limit=50
	app.GET("/users/:id", func(c flash.Ctx) error {
		id := c.ParamInt("id", 0) // 0 if missing/invalid
		active := c.QueryBool("active", false)
		limit := c.QueryInt("limit", 20) // default to 20

		return c.JSON(map[string]any{
			"id":     id,
			"active": active,
			"limit":  limit,
		})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
