package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		config         HealthCheckConfig
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "default health check",
			config: HealthCheckConfig{
				Path: "/health",
			},
			path:           "/health",
			expectedStatus: http.StatusOK,
			expectedBody:   `"status":"healthy"`,
		},
		{
			name: "custom path",
			config: HealthCheckConfig{
				Path: "/healthz",
			},
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   `"status":"healthy"`,
		},
		{
			name: "custom service name",
			config: HealthCheckConfig{
				Path:        "/health",
				ServiceName: "my-service",
			},
			path:           "/health",
			expectedStatus: http.StatusOK,
			expectedBody:   `"status":"healthy"`,
		},
		{
			name: "unhealthy with error",
			config: HealthCheckConfig{
				Path: "/health",
				HealthCheckFunc: func() error {
					return errors.New("database connection failed")
				},
			},
			path:           "/health",
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   `"status":"unhealthy"`,
		},
		{
			name: "different path should not match",
			config: HealthCheckConfig{
				Path: "/health",
			},
			path:           "/status",
			expectedStatus: http.StatusNotFound, // Should pass through to next handler
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := flash.New()
			RegisterHealthCheck(app, tt.config)

			// Add a handler for non-health check paths
			app.GET("/status", func(c flash.Ctx) error {
				return c.String(http.StatusNotFound, "not found")
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			app.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, w.Body.String(), tt.expectedBody)
			}
		})
	}
}

func TestHealthCheckWithPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		fn             HealthCheckFunc
		expectedStatus int
	}{
		{
			name:           "simple path",
			path:           "/health",
			fn:             nil,
			expectedStatus: http.StatusOK,
		},
		{
			name: "with health check function",
			path: "/health",
			fn: func() error {
				return errors.New("test error")
			},
			expectedStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := flash.New()
			RegisterHealthCheck(app, HealthCheckConfig{Path: tt.path, HealthCheckFunc: tt.fn})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			app.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHealthCheckWithPathHelper(t *testing.T) {
	// Test the HealthCheckWithPath helper function
	cfg := HealthCheckWithPath("/test")
	assert.Equal(t, "/test", cfg.Path)
	assert.Equal(t, "goflash", cfg.ServiceName)
	assert.Nil(t, cfg.HealthCheckFunc)

	// Test with function
	testFunc := func() error { return nil }
	cfg = HealthCheckWithPath("/test", testFunc)
	assert.Equal(t, "/test", cfg.Path)
	assert.NotNil(t, cfg.HealthCheckFunc)
}

func TestHealthCheckConfigDefaults(t *testing.T) {
	// Test that defaults are properly set
	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{}) // Empty config

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"service":"goflash"`)
	assert.Contains(t, body, `"status":"healthy"`)
}

func TestHealthCheckConfigCustomDefaults(t *testing.T) {
	// Test custom defaults
	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{
		ServiceName: "custom-service",
	}) // Only service name set

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"service":"custom-service"`)
	assert.Contains(t, body, `"status":"healthy"`)
}

func TestHealthCheckErrorHandling(t *testing.T) {
	var errorCalled bool
	var successCalled bool

	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{
		Path: "/health",
		HealthCheckFunc: func() error {
			return errors.New("test error")
		},
		OnErrorFunc: func(c flash.Ctx, err error) {
			errorCalled = true
			assert.Equal(t, "test error", err.Error())
		},
		OnSuccessFunc: func(c flash.Ctx) {
			successCalled = true
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.True(t, errorCalled)
	assert.False(t, successCalled)
	assert.Contains(t, w.Body.String(), `"error":"test error"`)
}

func TestHealthCheckSuccessHandling(t *testing.T) {
	var errorCalled bool
	var successCalled bool

	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{
		Path: "/health",
		HealthCheckFunc: func() error {
			return nil
		},
		OnErrorFunc: func(c flash.Ctx, err error) {
			errorCalled = true
		},
		OnSuccessFunc: func(c flash.Ctx) {
			successCalled = true
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, errorCalled)
	assert.True(t, successCalled)
	assert.Contains(t, w.Body.String(), `"status":"healthy"`)
}

func TestHealthCheckTimestampFormat(t *testing.T) {
	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{
		Path: "/health",
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.Bytes()

	// Unmarshal JSON and extract timestamp
	var resp struct {
		Timestamp string `json:"timestamp"`
	}
	err := json.Unmarshal(body, &resp)
	assert.NoError(t, err, "response should be valid JSON")
	_, err = time.Parse(time.RFC3339, resp.Timestamp)
	assert.NoError(t, err, "timestamp should be in RFC3339 format")
}

func TestHealthCheckMultiplePaths(t *testing.T) {
	app := flash.New()

	// Add health check at /health
	RegisterHealthCheck(app, HealthCheckConfig{
		Path: "/health",
	})

	// Add another health check at /healthz
	RegisterHealthCheck(app, HealthCheckConfig{
		Path:        "/healthz",
		ServiceName: "custom-service",
	})

	// Test both paths work
	paths := []string{"/health", "/healthz"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"healthy"`)
	}

	// Test that other paths are not affected
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHealthCheckWithMiddlewareChain(t *testing.T) {
	app := flash.New()

	// Add logging middleware
	app.Use(Logger())

	// Add health check middleware
	RegisterHealthCheck(app, HealthCheckConfig{
		Path: "/health",
	})

	// Add a regular route
	app.GET("/api", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "api response")
	})

	// Test health check works
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"healthy"`)

	// Test regular route still works
	req = httptest.NewRequest(http.MethodGet, "/api", nil)
	w = httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "api response", w.Body.String())
}
