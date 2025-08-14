package app

import (
	"path"
	"strings"
)

// cleanPath normalizes a URL path, ensuring it starts with '/' and is cleaned.
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
func joinPath(prefix, p string) string {
	if prefix == "" || prefix == "/" {
		return cleanPath(p)
	}
	if p == "" || p == "/" {
		return cleanPath(prefix)
	}
	return cleanPath(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(p, "/"))
}
