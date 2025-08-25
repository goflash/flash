package ctx

import (
	"net/http/httptest"
	"testing"

	router "github.com/julienschmidt/httprouter"
)

// TestSecurityHelpers tests the new security-focused parameter and query helpers
func TestSecurityHelpers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		method   string // "param" or "query"
		helper   string // "safe", "alphanum", "filename"
	}{
		// ParamSafe tests
		{
			name:     "ParamSafe escapes HTML",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
			method:   "param",
			helper:   "safe",
		},
		{
			name:     "ParamSafe escapes quotes",
			input:    `"dangerous"`,
			expected: "&#34;dangerous&#34;",
			method:   "param",
			helper:   "safe",
		},

		// QuerySafe tests
		{
			name:     "QuerySafe escapes HTML",
			input:    "<img src=x onerror=alert(1)>",
			expected: "&lt;img src=x onerror=alert(1)&gt;",
			method:   "query",
			helper:   "safe",
		},

		// ParamAlphaNum tests
		{
			name:     "ParamAlphaNum filters special chars",
			input:    "abc123!@#$%^&*()",
			expected: "abc123",
			method:   "param",
			helper:   "alphanum",
		},
		{
			name:     "ParamAlphaNum handles SQL injection attempt",
			input:    "user'; DROP TABLE users; --",
			expected: "userDROPTABLEusers",
			method:   "param",
			helper:   "alphanum",
		},

		// QueryAlphaNum tests
		{
			name:     "QueryAlphaNum filters special chars",
			input:    "search123!@#",
			expected: "search123",
			method:   "query",
			helper:   "alphanum",
		},

		// ParamFilename tests
		{
			name:     "ParamFilename allows valid filename",
			input:    "document.pdf",
			expected: "document.pdf",
			method:   "param",
			helper:   "filename",
		},
		{
			name:     "ParamFilename prevents path traversal",
			input:    "../../../etc/passwd",
			expected: ".....etcpasswd",
			method:   "param",
			helper:   "filename",
		},
		{
			name:     "ParamFilename removes hidden file prefix",
			input:    ".hidden.txt",
			expected: "hidden.txt",
			method:   "param",
			helper:   "filename",
		},
		{
			name:     "ParamFilename handles complex attack",
			input:    "..%2F..%2F..%2Fetc%2Fpasswd",
			expected: ".....etcpasswd",
			method:   "param",
			helper:   "filename",
		},

		// QueryFilename tests
		{
			name:     "QueryFilename prevents path traversal",
			input:    "../../../../secret.txt",
			expected: ".......secret.txt",
			method:   "query",
			helper:   "filename",
		},
		{
			name:     "QueryFilename allows safe filename",
			input:    "report_2024.xlsx",
			expected: "report_2024.xlsx",
			method:   "query",
			helper:   "filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock context
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)

			var params router.Params
			if tt.method == "param" {
				params = router.Params{{Key: "test", Value: tt.input}}
			} else {
				// For query tests, add the parameter to the URL
				q := r.URL.Query()
				q.Set("test", tt.input)
				r.URL.RawQuery = q.Encode()
			}

			ctx := &DefaultContext{}
			ctx.Reset(w, r, params, "/test")

			var result string
			switch tt.method + "_" + tt.helper {
			case "param_safe":
				result = ctx.ParamSafe("test")
			case "query_safe":
				result = ctx.QuerySafe("test")
			case "param_alphanum":
				result = ctx.ParamAlphaNum("test")
			case "query_alphanum":
				result = ctx.QueryAlphaNum("test")
			case "param_filename":
				result = ctx.ParamFilename("test")
			case "query_filename":
				result = ctx.QueryFilename("test")
			default:
				t.Fatalf("unknown helper: %s_%s", tt.method, tt.helper)
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestSecurityHelpersEmptyValues tests edge cases with empty values
func TestSecurityHelpersEmptyValues(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	ctx := &DefaultContext{}
	ctx.Reset(w, r, nil, "/test")

	// Test empty parameter
	if result := ctx.ParamSafe("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent param, got %q", result)
	}

	if result := ctx.ParamAlphaNum("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent param, got %q", result)
	}

	if result := ctx.ParamFilename("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent param, got %q", result)
	}

	// Test empty query
	if result := ctx.QuerySafe("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent query, got %q", result)
	}

	if result := ctx.QueryAlphaNum("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent query, got %q", result)
	}

	if result := ctx.QueryFilename("nonexistent"); result != "" {
		t.Errorf("expected empty string for nonexistent query, got %q", result)
	}
}

// TestSecurityHelpersUnicodeHandling tests how helpers handle Unicode characters
func TestSecurityHelpersUnicodeHandling(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/?test=café123", nil)
	params := router.Params{{Key: "test", Value: "café123"}}

	ctx := &DefaultContext{}
	ctx.Reset(w, r, params, "/test")

	// Unicode should be preserved in safe methods
	if result := ctx.ParamSafe("test"); result != "café123" {
		t.Errorf("expected 'café123', got %q", result)
	}

	if result := ctx.QuerySafe("test"); result != "café123" {
		t.Errorf("expected 'café123', got %q", result)
	}

	// Unicode should be filtered out in alphanumeric methods
	if result := ctx.ParamAlphaNum("test"); result != "caf123" {
		t.Errorf("expected 'caf123', got %q", result)
	}

	if result := ctx.QueryAlphaNum("test"); result != "caf123" {
		t.Errorf("expected 'caf123', got %q", result)
	}

	// Unicode should be filtered out in filename methods
	if result := ctx.ParamFilename("test"); result != "caf123" {
		t.Errorf("expected 'caf123', got %q", result)
	}

	if result := ctx.QueryFilename("test"); result != "caf123" {
		t.Errorf("expected 'caf123', got %q", result)
	}
}
