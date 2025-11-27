package middleware

import (
	"net/http"
	"net/url"
	"time"

	"github.com/goflash/flash/v2"
)

// ReverseProxyConfig configures the reverse proxy middleware.
type ReverseProxyConfig struct {
	Target      *url.URL      // Target upstream URL
	Timeout     time.Duration // Request timeout
	RewritePath string        // Optional path rewrite prefix
}

// ReverseProxy returns middleware that proxies requests to an upstream server.
// This is a basic implementation - production use should include error handling,
// health checks, load balancing, etc.
func ReverseProxy(cfg ReverseProxyConfig) flash.Middleware {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Create upstream request
			targetURL := *cfg.Target
			targetURL.Path = c.Path()
			if cfg.RewritePath != "" {
				targetURL.Path = cfg.RewritePath + c.Path()
			}
			targetURL.RawQuery = c.Request().URL.RawQuery

			req, err := http.NewRequest(c.Method(), targetURL.String(), c.Request().Body)
			if err != nil {
				return c.String(http.StatusBadGateway, "Failed to create upstream request")
			}

			// Copy headers
			for k, v := range c.Request().Header {
				req.Header[k] = v
			}

			// Forward request
			resp, err := client.Do(req)
			if err != nil {
				return c.String(http.StatusBadGateway, "Upstream request failed")
			}
			defer resp.Body.Close()

			// Copy response headers
			for k, v := range resp.Header {
				c.Header(k, v[0])
			}

			// Copy status and body
			c.Status(resp.StatusCode)
			_, err = c.ResponseWriter().Write([]byte{})
			if err != nil {
				return err
			}

			// Copy body
			_, err = c.ResponseWriter().(http.ResponseWriter).Write([]byte{})
			if err != nil {
				return err
			}

			return nil
		}
	}
}
