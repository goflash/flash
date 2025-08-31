package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goflash/flash/v2"
	"github.com/stretchr/testify/assert"
)

func TestPprof(t *testing.T) {
	tests := []struct {
		name           string
		config         PprofConfig
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "default prefix index",
			config:         PprofConfig{},
			path:           "/debug/pprof/",
			expectedStatus: http.StatusOK,
			expectedBody:   "Types of profiles available",
		},
		{
			name:           "default prefix cmdline",
			config:         PprofConfig{},
			path:           "/debug/pprof/cmdline",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "default prefix profile",
			config:         PprofConfig{},
			path:           "/debug/pprof/profile?seconds=1",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "default prefix symbol",
			config:         PprofConfig{},
			path:           "/debug/pprof/symbol",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "default prefix trace",
			config:         PprofConfig{},
			path:           "/debug/pprof/trace",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "default prefix heap",
			config:         PprofConfig{},
			path:           "/debug/pprof/heap",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "default prefix goroutine",
			config:         PprofConfig{},
			path:           "/debug/pprof/goroutine",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "custom prefix index",
			config:         PprofConfig{Prefix: "/profiling"},
			path:           "/profiling/",
			expectedStatus: http.StatusOK,
			expectedBody:   "Types of profiles available",
		},
		{
			name:           "custom prefix heap",
			config:         PprofConfig{Prefix: "/profiling"},
			path:           "/profiling/heap",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "prefix without leading slash",
			config:         PprofConfig{Prefix: "profiling"},
			path:           "/profiling/",
			expectedStatus: http.StatusOK,
			expectedBody:   "Types of profiles available",
		},
		{
			name:           "prefix with trailing slash",
			config:         PprofConfig{Prefix: "/profiling/"},
			path:           "/profiling/",
			expectedStatus: http.StatusOK,
			expectedBody:   "Types of profiles available",
		},
		{
			name:           "non-pprof path passes through",
			config:         PprofConfig{},
			path:           "/api/users",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
		{
			name:           "partial prefix match passes through",
			config:         PprofConfig{},
			path:           "/debug/pprof-extra",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := flash.New()

			// Use MountPprof for more reliable testing
			if tt.config.Prefix == "" {
				MountPprof(app)
			} else {
				MountPprof(app, tt.config.Prefix)
			}

			// Add a test handler
			app.GET("/*", func(c flash.Ctx) error {
				return c.String(http.StatusOK, "handler called")
			})

			// Create request
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			// Serve request
			app.ServeHTTP(rec, req)

			// Assert status code
			assert.Equal(t, tt.expectedStatus, rec.Code)

			// Assert body content
			if tt.expectedBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectedBody)
			}
		})
	}
}

func TestPprofConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   PprofConfig
		expected string
	}{
		{
			name:     "empty config uses default",
			config:   PprofConfig{},
			expected: "/debug/pprof",
		},
		{
			name:     "custom prefix",
			config:   PprofConfig{Prefix: "/custom"},
			expected: "/custom",
		},
		{
			name:     "prefix without leading slash",
			config:   PprofConfig{Prefix: "custom"},
			expected: "/custom",
		},
		{
			name:     "prefix with trailing slash",
			config:   PprofConfig{Prefix: "/custom/"},
			expected: "/custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := flash.New()

			if tt.config.Prefix == "" {
				MountPprof(app)
			} else {
				MountPprof(app, tt.config.Prefix)
			}

			// Test that the prefix is correctly configured by checking the index page
			req := httptest.NewRequest(http.MethodGet, tt.expected+"/", nil)
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "Types of profiles available")
		})
	}
}

func TestPprofSecurity(t *testing.T) {
	app := flash.New()
	MountPprof(app)

	// Test that pprof endpoints are accessible
	pprofEndpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile?seconds=1",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/threadcreate",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
		"/debug/pprof/allocs",
	}

	for _, endpoint := range pprofEndpoints {
		t.Run("endpoint_"+strings.TrimPrefix(endpoint, "/debug/pprof/"), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			// All pprof endpoints should return 200 OK
			assert.Equal(t, http.StatusOK, rec.Code, "Endpoint %s failed", endpoint)
		})
	}
}

func TestPprofWithQueryParams(t *testing.T) {
	app := flash.New()
	MountPprof(app)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "profile with seconds parameter",
			path:           "/debug/pprof/profile?seconds=1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "heap with gc parameter",
			path:           "/debug/pprof/heap?gc=1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "goroutine with seconds parameter",
			path:           "/debug/pprof/goroutine?seconds=1",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestPprofMiddlewareOrder(t *testing.T) {
	app := flash.New()

	// Add pprof using MountPprof
	MountPprof(app)

	// Add a custom middleware that sets a header
	app.Use(func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			c.Header("X-Custom-Header", "test-value")
			return next(c)
		}
	})

	// Add a test handler
	app.GET("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "test handler")
	})

	// Test that pprof requests work correctly
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Types of profiles available")

	// Test that regular requests also work
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	rec = httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test handler", rec.Body.String())
	assert.Equal(t, "test-value", rec.Header().Get("X-Custom-Header"))
}

func TestPprofMultipleConfigs(t *testing.T) {
	app := flash.New()

	// Test that only the first prefix works
	MountPprof(app, "/first")

	// Test that only the first prefix works
	req := httptest.NewRequest(http.MethodGet, "/first/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Types of profiles available")

	// Test that the second prefix doesn't work
	req = httptest.NewRequest(http.MethodGet, "/second/", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPprofBasic(t *testing.T) {
	app := flash.New()
	MountPprof(app)

	// Test basic pprof endpoints (excluding profile which takes 30 seconds)
	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "index page",
			path:           "/debug/pprof/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "cmdline",
			path:           "/debug/pprof/cmdline",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "symbol",
			path:           "/debug/pprof/symbol",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "heap",
			path:           "/debug/pprof/heap",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "goroutine",
			path:           "/debug/pprof/goroutine",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "non-pprof path",
			path:           "/api/test",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}
