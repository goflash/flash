package middleware

import (
	"net/http"

	"github.com/goflash/flash/v2"
	"golang.org/x/net/websocket"
)

// WebSocketUpgrade upgrades an HTTP connection to a WebSocket connection.
// It uses golang.org/x/net/websocket for the upgrade.
func WebSocketUpgrade(handler func(*websocket.Conn)) flash.Handler {
	return func(c flash.Ctx) error {
		ws := websocket.Server{
			Handler: handler,
		}
		ws.ServeHTTP(c.ResponseWriter(), c.Request())
		return nil
	}
}

// SSEStream provides Server-Sent Events streaming support.
// It sets appropriate headers and provides a helper to write SSE events.
func SSEStream(c flash.Ctx) (*SSEWriter, error) {
	w := c.ResponseWriter()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	
	return &SSEWriter{w: w}, nil
}

// SSEWriter provides methods to write SSE events.
type SSEWriter struct {
	w http.ResponseWriter
}

// WriteEvent writes an SSE event with the given event type and data.
func (s *SSEWriter) WriteEvent(event, data string) error {
	if event != "" {
		_, err := s.w.Write([]byte("event: " + event + "\n"))
		if err != nil {
			return err
		}
	}
	_, err := s.w.Write([]byte("data: " + data + "\n\n"))
	if err != nil {
		return err
	}
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// WriteData writes SSE data without an event type.
func (s *SSEWriter) WriteData(data string) error {
	return s.WriteEvent("", data)
}

