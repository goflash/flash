package middleware

import (
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

func TestHealthCheckDefaults(t *testing.T) {
	app := flash.New()
	RegisterHealthCheck(app, HealthCheckConfig{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"status":"healthy"`)
	assert.Contains(t, body, `"service":"goflash"`)
	assert.Contains(t, body, `"timestamp":"`)
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
	body := w.Body.String()

	// Extract timestamp from response
	start := len(`{"service":"goflash","status":"healthy","timestamp":"`)
	end := start + len(time.RFC3339)
	if len(body) >= end {
		timestamp := body[start:end]
		_, err := time.Parse(time.RFC3339, timestamp)
		assert.NoError(t, err, "timestamp should be in RFC3339 format")
	}
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
