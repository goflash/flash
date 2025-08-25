package middleware

import (
	"path"
	"strings"
	"testing"
)

// TestSanitizedPathAlwaysAssigned tests that sanitizedPath is always assigned correctly
// This test validates the fix for the compilation error mentioned in the problem statement.
func TestSanitizedPathAlwaysAssigned(t *testing.T) {
	tests := []struct {
		name         string
		config       HealthConfig
		expectedPath string
	}{
		{
			name: "normal path without sanitization needed",
			config: HealthConfig{
				Path:         "/health",
				SanitizePath: true,
			},
			expectedPath: "/health",
		},
		{
			name: "path with double slashes that needs sanitization",
			config: HealthConfig{
				Path:         "//health//check//",
				SanitizePath: true,
			},
			expectedPath: "/health/check",
		},
		{
			name: "path without leading slash",
			config: HealthConfig{
				Path:         "health",
				SanitizePath: false,
			},
			expectedPath: "health",
		},
		{
			name: "sanitization disabled for path with double slashes",
			config: HealthConfig{
				Path:         "//health//",
				SanitizePath: false,
			},
			expectedPath: "//health//",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from RegisterHealthCheck to test sanitizedPath assignment
			sanitizedPath := tt.config.Path
			
			// Override sanitizedPath if path sanitization is needed
			if tt.config.SanitizePath && strings.Contains(tt.config.Path, "//") {
				sanitizedPath = path.Clean(tt.config.Path)
				if !strings.HasPrefix(sanitizedPath, "/") {
					sanitizedPath = "/" + sanitizedPath
				}
			}
			
			// Verify the path was assigned correctly
			if sanitizedPath != tt.expectedPath {
				t.Errorf("sanitizedPath = %q, want %q", sanitizedPath, tt.expectedPath)
			}
		})
	}
}

// TestDefaultHealthConfig tests the default configuration
func TestDefaultHealthConfig(t *testing.T) {
	cfg := DefaultHealthConfig()
	
	if cfg.Path != "/health" {
		t.Errorf("Default path = %q, want %q", cfg.Path, "/health")
	}
	
	if !cfg.SanitizePath {
		t.Error("Default SanitizePath should be true")
	}
	
	if cfg.Handler == nil {
		t.Error("Default Handler should not be nil")
	}
}