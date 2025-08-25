package security

import (
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid simple path",
			input:    "/health",
			expected: "/health",
		},
		{
			name:     "valid path with underscores",
			input:    "/health_check",
			expected: "/health_check",
		},
		{
			name:     "valid path with hyphens",
			input:    "/health-check",
			expected: "/health-check",
		},
		{
			name:     "valid path with dots",
			input:    "/health.v1",
			expected: "/health.v1",
		},
		{
			name:     "valid path with tildes",
			input:    "/health~check",
			expected: "/health~check",
		},
		{
			name:     "path without leading slash",
			input:    "health",
			expected: "/health",
		},
		{
			name:     "path with double slashes",
			input:    "//health",
			expected: "/health",
		},
		{
			name:     "path with dot segments",
			input:    "/health/../status",
			expected: "/status",
		},
		{
			name:     "path with current directory",
			input:    "/health/./status",
			expected: "/health/status",
		},
		{
			name:     "URL encoded path",
			input:    "/health%20check",
			expected: "/health check",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "/",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "/",
		},
		{
			name:     "invalid characters - spaces",
			input:    "/health check",
			expected: "/health check",
		},
		{
			name:     "invalid characters - special chars",
			input:    "/health@check",
			expected: "",
		},
		{
			name:     "invalid characters - unicode",
			input:    "/health✓check",
			expected: "",
		},
		{
			name:     "invalid URL encoding",
			input:    "/health%",
			expected: "",
		},
		{
			name:     "path with multiple segments",
			input:    "/api/v1/health",
			expected: "/api/v1/health",
		},
		{
			name:     "complex path with all valid characters",
			input:    "/api/v1/health_check-status.v2~test",
			expected: "/api/v1/health_check-status.v2~test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePath(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
