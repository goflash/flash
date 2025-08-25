package app

import (
	"path"
	"strings"
)

// cleanPath normalizes a URL path for route registration and mounting.
// It ensures the path starts with '/' and applies path.Clean to collapse
// duplicates (e.g., "/api//v1/" -> "/api/v1").
//
// Special cases:
//   - Empty input returns "/"
//   - The result never ends with a trailing slash unless it is the root "/"
//
// Examples:
//
//	cleanPath("")          // "/"
//	cleanPath("users")     // "/users"
//	cleanPath("/api//v1/") // "/api/v1"
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// joinPath joins two URL path segments, cleaning the result.
// It handles empty or root segments gracefully and ensures a single slash
// joins the parts.
//
// Examples:
//
//	joinPath("/api", "/v1")   // "/api/v1"
//	joinPath("/api/", "v1")   // "/api/v1"
//	joinPath("/", "users")    // "/users"
//	joinPath("/admin", "/")   // "/admin"
func joinPath(prefix, p string) string {
	if prefix == "" || prefix == "/" {
		return cleanPath(p)
	}
	if p == "" || p == "/" {
		return cleanPath(prefix)
	}
	return cleanPath(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(p, "/"))
}
