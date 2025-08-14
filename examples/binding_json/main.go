package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	"github.com/goflash/flash/validate"
)

// UserIn represents the input user payload for binding example.
type UserIn struct {
	Name string `json:"name"` // Name is the user's name.
	Age  int    `json:"age"`  // Age is the user's age.
}

// UserOut represents the output user payload for binding example.
type UserOut struct {
	ID   int    `json:"id"`   // ID is the user's ID.
	Name string `json:"name"` // Name is the user's name.
	Age  int    `json:"age"`  // Age is the user's age.
}

// main demonstrates JSON binding and response in flash.
func main() {
	app := flash.New()

	// POST /users binds JSON input and returns a JSON response.
	app.POST("/users", func(c *flash.Ctx) error {
		var in UserIn
		if err := c.BindJSON(&in); err != nil {
			// return c.String(http.StatusBadRequest, "invalid json")

			// Alternatively, you could return a structured error response:
			return c.Status(http.StatusUnprocessableEntity).JSON(map[string]any{
				"message": "validation failed",
				"fields":  validate.ToFieldErrors(err),
			})
		}
		return c.JSON(UserOut{ID: 1, Name: in.Name, Age: in.Age})
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
