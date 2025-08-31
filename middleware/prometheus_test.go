package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusMiddleware(t *testing.T) {
	app := flash.New()

	// Apply Prometheus middleware and mount metrics endpoint
	MountPrometheus(app)

	// Add test routes
	app.GET("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	app.GET("/slow", func(c flash.Ctx) error {
		time.Sleep(10 * time.Millisecond)
		return c.String(http.StatusOK, "slow")
	})

	app.POST("/data", func(c flash.Ctx) error {
		return c.JSON(map[string]string{"status": "created"})
	})

	app.GET("/error", func(c flash.Ctx) error {
		return c.String(http.StatusInternalServerError, "error")
	})

	// Test basic request tracking
	t.Run("basic_request_tracking", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	// Test metrics endpoint
	t.Run("metrics_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()

		// Check that metrics are exposed
		assert.Contains(t, body, "# HELP http_requests_requests_total")
		assert.Contains(t, body, "# TYPE http_requests_requests_total counter")
		assert.Contains(t, body, "# HELP http_requests_request_duration_seconds")
		assert.Contains(t, body, "# TYPE http_requests_request_duration_seconds histogram")
	})

	// Test request duration tracking
	t.Run("request_duration_tracking", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/slow", nil)
		rec := httptest.NewRecorder()

		start := time.Now()
		app.ServeHTTP(rec, req)
		duration := time.Since(start)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.GreaterOrEqual(t, duration, 10*time.Millisecond)
	})

	// Test different HTTP methods
	t.Run("different_methods", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(`{"test":"data"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "created")
	})

	// Test error status tracking
	t.Run("error_status_tracking", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/error", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Equal(t, "error", rec.Body.String())
	})
}

func TestPrometheusWithConfig(t *testing.T) {
	app := flash.New()

	// Apply Prometheus middleware with custom config and mount metrics endpoint
	config := PrometheusConfig{
		Namespace:     "custom",
		Subsystem:     "api",
		MetricsPath:   "/custom-metrics",
		ExposeMetrics: true,
	}
	MountPrometheus(app, config)

	app.GET("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	// First make a request to generate some metrics
	t.Run("generate_metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	// Test custom metrics endpoint
	t.Run("custom_metrics_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/custom-metrics", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()

		// Check that custom namespace and subsystem are used
		assert.Contains(t, body, "# HELP custom_api_requests_total")
		assert.Contains(t, body, "# TYPE custom_api_requests_total counter")
		assert.Contains(t, body, "# HELP custom_api_request_duration_seconds")
		assert.Contains(t, body, "# TYPE custom_api_request_duration_seconds histogram")
	})

	// Test that default metrics endpoint is not available
	t.Run("default_metrics_not_available", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestMountPrometheus(t *testing.T) {
	app := flash.New()

	// Use MountPrometheus function
	MountPrometheus(app)

	app.GET("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	// First make a request to generate metrics
	t.Run("generate_metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	// Test that metrics endpoint is available
	t.Run("mount_prometheus_metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()

		// Check that metrics are exposed
		assert.Contains(t, body, "# HELP http_requests_requests_total")
		assert.Contains(t, body, "# TYPE http_requests_requests_total counter")
	})

	// Test that middleware is applied
	t.Run("mount_prometheus_middleware_applied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})
}

func TestPrometheusWithoutMetricsEndpoint(t *testing.T) {
	app := flash.New()

	// Apply Prometheus middleware without exposing metrics endpoint
	config := PrometheusConfig{
		ExposeMetrics: false,
	}
	app.Use(PrometheusWithConfig(config))

	app.GET("/test", func(c flash.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	// Test that metrics endpoint is not available
	t.Run("no_metrics_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	// Test that middleware still works
	t.Run("middleware_still_works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})
}
