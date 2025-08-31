package middleware

import (
	"net/http/pprof"
	"strings"

	"github.com/goflash/flash/v2"
)

// PprofConfig configures the Pprof middleware.
type PprofConfig struct {
	// Prefix sets the URL prefix for pprof endpoints (default: /debug/pprof).
	// The middleware will mount all pprof handlers under this prefix.
	Prefix string
}

// Pprof returns middleware that mounts Go's built-in pprof handlers under a configurable prefix.
// This middleware provides runtime profiling data in the format expected by the pprof visualization tool.
//
// The middleware mounts the following endpoints under the configured prefix:
//   - / (index page showing all available profiles)
//   - /cmdline (command line arguments)
//   - /profile (CPU profile)
//   - /symbol (symbol lookup)
//   - /trace (execution trace)
//   - /heap (heap profile)
//   - /goroutine (goroutine profile)
//   - /threadcreate (thread creation profile)
//   - /block (blocking profile)
//   - /mutex (mutex contention profile)
//   - /allocs (allocation profile)
//
// Usage examples:
//   - View all profiles: http://localhost:8080/debug/pprof/
//   - CPU profile: http://localhost:8080/debug/pprof/profile?seconds=30
//   - Heap profile: http://localhost:8080/debug/pprof/heap
//   - Goroutine profile: http://localhost:8080/debug/pprof/goroutine
//
// Security considerations:
//   - This middleware exposes sensitive runtime information
//   - Should only be enabled in development or controlled environments
//   - Consider adding authentication/authorization if exposed publicly
//   - The prefix should be chosen carefully to avoid conflicts with application routes
//
// Example:
//
//	// Mount pprof under default /debug/pprof prefix
//	app.Use(Pprof())
//
//	// Mount pprof under custom prefix
//	app.Use(Pprof(middleware.PprofConfig{
//		Prefix: "/profiling",
//	}))
func Pprof(cfgs ...PprofConfig) flash.Middleware {
	cfg := PprofConfig{Prefix: "/debug/pprof"}
	if len(cfgs) > 0 && cfgs[0].Prefix != "" {
		cfg.Prefix = cfgs[0].Prefix
	}

	// Ensure prefix starts with /
	if !strings.HasPrefix(cfg.Prefix, "/") {
		cfg.Prefix = "/" + cfg.Prefix
	}

	// Remove trailing slash if present
	cfg.Prefix = strings.TrimSuffix(cfg.Prefix, "/")

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			path := c.Path()

			// Check if the request is for a pprof endpoint
			if strings.HasPrefix(path, cfg.Prefix) {
				// Extract the pprof path by removing our prefix
				pprofPath := strings.TrimPrefix(path, cfg.Prefix)
				if pprofPath == "" {
					pprofPath = "/"
				}

				// Handle pprof requests
				switch pprofPath {
				case "/":
					// Index page showing all available profiles
					pprof.Index(c.ResponseWriter(), c.Request())
					return nil

				case "/cmdline":
					// Command line arguments
					pprof.Cmdline(c.ResponseWriter(), c.Request())
					return nil

				case "/profile":
					// CPU profile
					pprof.Profile(c.ResponseWriter(), c.Request())
					return nil

				case "/symbol":
					// Symbol lookup
					pprof.Symbol(c.ResponseWriter(), c.Request())
					return nil

				case "/trace":
					// Execution trace
					pprof.Trace(c.ResponseWriter(), c.Request())
					return nil

				default:
					// Handle other pprof profiles (heap, goroutine, etc.)
					// Extract profile name from path
					profileName := strings.TrimPrefix(pprofPath, "/")
					if profileName != "" {
						pprof.Handler(profileName).ServeHTTP(c.ResponseWriter(), c.Request())
						return nil
					}
				}
			}

			// Not a pprof request, continue to next handler
			return next(c)
		}
	}
}

// MountPprof is an alternative function that directly mounts pprof routes to an app.
// This is more efficient than the middleware approach for pprof endpoints.
//
// Example:
//
//	app := flash.New()
//	MountPprof(app) // Mounts under /debug/pprof
//	MountPprof(app, "/profiling") // Mounts under /profiling
func MountPprof(app flash.App, prefix ...string) {
	pprofPrefix := "/debug/pprof"
	if len(prefix) > 0 && prefix[0] != "" {
		pprofPrefix = prefix[0]
		if !strings.HasPrefix(pprofPrefix, "/") {
			pprofPrefix = "/" + pprofPrefix
		}
		pprofPrefix = strings.TrimSuffix(pprofPrefix, "/")
	}

	// Mount all pprof endpoints
	app.GET(pprofPrefix+"/", func(c flash.Ctx) error {
		pprof.Index(c.ResponseWriter(), c.Request())
		return nil
	})

	app.GET(pprofPrefix+"/cmdline", func(c flash.Ctx) error {
		pprof.Cmdline(c.ResponseWriter(), c.Request())
		return nil
	})

	app.GET(pprofPrefix+"/profile", func(c flash.Ctx) error {
		pprof.Profile(c.ResponseWriter(), c.Request())
		return nil
	})

	app.GET(pprofPrefix+"/symbol", func(c flash.Ctx) error {
		pprof.Symbol(c.ResponseWriter(), c.Request())
		return nil
	})

	app.GET(pprofPrefix+"/trace", func(c flash.Ctx) error {
		pprof.Trace(c.ResponseWriter(), c.Request())
		return nil
	})

	// Mount dynamic profile handlers
	profiles := []string{"heap", "goroutine", "threadcreate", "block", "mutex", "allocs"}
	for _, profile := range profiles {
		profileName := profile
		app.GET(pprofPrefix+"/"+profile, func(c flash.Ctx) error {
			pprof.Handler(profileName).ServeHTTP(c.ResponseWriter(), c.Request())
			return nil
		})
	}
}
