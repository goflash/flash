package main

import (
	"log"
	"net/http"

	"github.com/goflash/flash"
	"github.com/gorilla/websocket"
)

// upgrader is a WebSocket upgrader with permissive origin check for demo purposes.
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// main demonstrates WebSocket upgrade and echo using goflash and gorilla/websocket.
func main() {
	app := flash.New()

	// GET /ws upgrades the connection to WebSocket and echoes messages.
	app.GET("/ws", func(c *flash.Ctx) error {
		// If this is not a WebSocket upgrade request, return a clear 400 without invoking the upgrader.
		if !websocket.IsWebSocketUpgrade(c.Request()) {
			return c.String(http.StatusBadRequest, "WebSocket endpoint. Connect using a WS client.")
		}

		conn, err := upgrader.Upgrade(c.ResponseWriter(), c.Request(), nil)
		if err != nil {
			// Upgrader may have already written the error response (e.g., 400), so don't write again.
			return nil
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return nil
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				return nil
			}
		}
	})

	log.Fatal(http.ListenAndServe(":8080", app))
}
