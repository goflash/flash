package security

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// allow only safe URL path characters (RFC 3986 + common web safe set)
var safePathRegex = regexp.MustCompile(`^[a-zA-Z0-9/_\-\.\~ ]*$`)

// SanitizePath normalizes, decodes, and validates a request path.
// Returns "" if invalid.
func SanitizePath(rawPath string) string {
	// Handle empty string
	if rawPath == "" {
		return "/"
	}

	// 1. Decode % escapes
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return ""
	}
	// 2. Clean (remove ../, //, ./)
	clean := path.Clean(decoded)
	// 3. Force leading slash (avoid escaping root)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	// 4. Validate characters
	if !safePathRegex.MatchString(clean) {
		return ""
	}
	return clean
}
