package middleware

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/goflash/flash/v2"
)

// DecompressConfig configures the decompression middleware.
type DecompressConfig struct {
	// AllowedEncodings specifies which encodings to support.
	// Defaults to ["gzip", "deflate"] if empty.
	AllowedEncodings []string
}

// Decompress returns middleware that automatically decompresses request bodies
// when the Content-Encoding header indicates compression (gzip or deflate).
// The decompressed body is transparently available to handlers via BindJSON and other body readers.
func Decompress(cfgs ...DecompressConfig) flash.Middleware {
	cfg := DecompressConfig{
		AllowedEncodings: []string{"gzip", "deflate"},
	}
	if len(cfgs) > 0 {
		c := cfgs[0]
		if len(c.AllowedEncodings) > 0 {
			cfg.AllowedEncodings = c.AllowedEncodings
		}
	}

	// Build a map for fast lookup
	allowed := make(map[string]bool, len(cfg.AllowedEncodings))
	for _, enc := range cfg.AllowedEncodings {
		allowed[strings.ToLower(enc)] = true
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			r := c.Request()
			encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))

			// No compression or not allowed encoding
			if encoding == "" || !allowed[encoding] {
				return next(c)
			}

			// Only decompress if there's a body
			if r.Body == nil || r.Body == http.NoBody {
				return next(c)
			}

			var reader io.ReadCloser
			var err error

			switch encoding {
			case "gzip":
				reader, err = gzip.NewReader(r.Body)
				if err != nil {
					return c.Status(http.StatusBadRequest).String(http.StatusBadRequest, "Invalid gzip content")
				}
			case "deflate":
				reader = flate.NewReader(r.Body)
			default:
				return next(c)
			}

			// Replace the request body with the decompressed reader
			r.Body = reader
			c.SetRequest(r)

			return next(c)
		}
	}
}
