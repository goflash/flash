package middleware

import (
	"net/http"

	"github.com/goflash/flash/v2"
)

// OpenAPIConfig configures the OpenAPI/Swagger UI middleware.
type OpenAPIConfig struct {
	SpecPath string // Path to serve OpenAPI spec (e.g., "/openapi.json")
	UIPath   string // Path to serve Swagger UI (e.g., "/swagger")
	Spec     []byte // OpenAPI spec content (JSON)
}

// OpenAPI returns middleware that serves OpenAPI spec and Swagger UI.
// This is a placeholder implementation - full implementation would require
// embedding Swagger UI assets and serving the OpenAPI spec.
func OpenAPI(cfg OpenAPIConfig) flash.Middleware {
	if cfg.SpecPath == "" {
		cfg.SpecPath = "/openapi.json"
	}
	if cfg.UIPath == "" {
		cfg.UIPath = "/swagger"
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			path := c.Path()

			// Serve OpenAPI spec
			if path == cfg.SpecPath {
				c.Header("Content-Type", "application/json")
				return c.Send(http.StatusOK, "application/json", cfg.Spec)
			}

			// Serve Swagger UI placeholder
			if path == cfg.UIPath {
				html := `<!DOCTYPE html>
<html>
<head>
	<title>Swagger UI</title>
	<link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css" />
</head>
<body>
	<div id="swagger-ui"></div>
	<script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
	<script>
		SwaggerUIBundle({
			url: "` + cfg.SpecPath + `",
			dom_id: "#swagger-ui"
		});
	</script>
</body>
</html>`
				c.Header("Content-Type", "text/html")
				return c.Send(http.StatusOK, "text/html", []byte(html))
			}

			return next(c)
		}
	}
}
