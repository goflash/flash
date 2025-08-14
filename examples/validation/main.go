package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	"github.com/goflash/flash/validate"
)

// User represents a user for the validation example.
type User struct {
	Name string `json:"name" validate:"required,min=2"` // min 2 chars
	Age  int    `json:"age"  validate:"gte=0,lte=130"`  // 0-130
}

// main starts a goflash web server with a POST /users endpoint that validates input.
func main() {
	app := flash.New()

	// POST /users accepts a JSON user and validates fields using framework validation.
	app.POST("/users", func(c *flash.Ctx) error {
		var u User
		if err := c.BindJSON(&u); err != nil {
			return c.Status(http.StatusUnprocessableEntity).JSON(map[string]any{
				"message":        "invalid payload structure",
				"fields":         validate.ToFieldErrors(err),
				"original_error": err.Error(),
			})
		}
		if err := validate.Struct(u); err != nil {
			return c.Status(http.StatusUnprocessableEntity).JSON(map[string]any{
				"message":        "validation failed",
				"fields":         validate.ToFieldErrors(err),
				"original_error": err.Error(),
			})
		}
		return c.JSON(u)
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
