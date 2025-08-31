package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusConfig configures the Prometheus middleware
type PrometheusConfig struct {
	// Namespace for all metrics (default: "http")
	Namespace string
	// Subsystem for all metrics (default: "requests")
	Subsystem string
	// Path to expose metrics (default: "/metrics")
	MetricsPath string
	// Whether to expose metrics endpoint (default: true)
	ExposeMetrics bool
	// Custom labels to add to all metrics
	Labels map[string]string
}

// DefaultPrometheusConfig returns the default configuration
func DefaultPrometheusConfig() PrometheusConfig {
	return PrometheusConfig{
		Namespace:     "http",
		Subsystem:     "requests",
		MetricsPath:   "/metrics",
		ExposeMetrics: true,
		Labels:        make(map[string]string),
	}
}

// prometheusMetrics holds the metrics for a specific configuration
type prometheusMetrics struct {
	requestCounter   *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestSize      *prometheus.HistogramVec
	responseSize     *prometheus.HistogramVec
	requestsInFlight *prometheus.GaugeVec
	registry         *prometheus.Registry
}

// newPrometheusMetrics creates metrics for a specific configuration
func newPrometheusMetrics(cfg PrometheusConfig) *prometheusMetrics {
	registry := prometheus.NewRegistry()

	requestCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	requestSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "request_size_bytes",
			Help:      "HTTP request size in bytes",
			Buckets:   []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"method", "path"},
	)

	responseSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "response_size_bytes",
			Help:      "HTTP response size in bytes",
			Buckets:   []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"method", "path"},
	)

	requestsInFlight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "requests_in_flight",
			Help:      "Current number of HTTP requests being processed",
		},
		[]string{"method", "path"},
	)

	// Register all metrics with the registry
	registry.MustRegister(requestCounter, requestDuration, requestSize, responseSize, requestsInFlight)

	return &prometheusMetrics{
		requestCounter:   requestCounter,
		requestDuration:  requestDuration,
		requestSize:      requestSize,
		responseSize:     responseSize,
		requestsInFlight: requestsInFlight,
		registry:         registry,
	}
}

// Prometheus creates a new Prometheus middleware with default configuration
func Prometheus() flash.Middleware {
	return PrometheusWithConfig(DefaultPrometheusConfig())
}

// PrometheusWithConfig creates a new Prometheus middleware with custom configuration
func PrometheusWithConfig(cfg PrometheusConfig) flash.Middleware {
	metrics := newPrometheusMetrics(cfg)

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			start := time.Now()
			method := c.Method()
			path := c.Path()

			// Increment in-flight requests
			metrics.requestsInFlight.WithLabelValues(method, path).Inc()
			defer metrics.requestsInFlight.WithLabelValues(method, path).Dec()

			// Record request size if available
			if c.Request() != nil && c.Request().ContentLength > 0 {
				metrics.requestSize.WithLabelValues(method, path).Observe(float64(c.Request().ContentLength))
			}

			// Create a response writer wrapper to capture response size
			responseWriter := &responseSizeWriter{
				ResponseWriter: c.ResponseWriter(),
				method:         method,
				path:           path,
				responseSize:   metrics.responseSize,
			}
			c.SetResponseWriter(responseWriter)

			// Process the request
			err := next(c)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(c.StatusCode())

			metrics.requestCounter.WithLabelValues(method, path, status).Inc()
			metrics.requestDuration.WithLabelValues(method, path).Observe(duration)

			return err
		}
	}
}

// responseSizeWriter wraps http.ResponseWriter to capture response size
type responseSizeWriter struct {
	http.ResponseWriter
	method       string
	path         string
	responseSize *prometheus.HistogramVec
	written      int
}

func (w *responseSizeWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

func (w *responseSizeWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.Write([]byte(s))
	w.written += n
	return n, err
}

func (w *responseSizeWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// prometheusApp holds the app and metrics for mounting
type prometheusApp struct {
	app     flash.App
	metrics *prometheusMetrics
	config  PrometheusConfig
}

// MountPrometheus mounts Prometheus metrics endpoint and optionally applies middleware
func MountPrometheus(app flash.App, cfg ...PrometheusConfig) {
	var config PrometheusConfig
	if len(cfg) > 0 {
		config = cfg[0]
	} else {
		config = DefaultPrometheusConfig()
	}

	metrics := newPrometheusMetrics(config)

	// Apply Prometheus middleware to the app
	app.Use(func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			start := time.Now()
			method := c.Method()
			path := c.Path()

			// Increment in-flight requests
			metrics.requestsInFlight.WithLabelValues(method, path).Inc()
			defer metrics.requestsInFlight.WithLabelValues(method, path).Dec()

			// Record request size if available
			if c.Request() != nil && c.Request().ContentLength > 0 {
				metrics.requestSize.WithLabelValues(method, path).Observe(float64(c.Request().ContentLength))
			}

			// Create a response writer wrapper to capture response size
			responseWriter := &responseSizeWriter{
				ResponseWriter: c.ResponseWriter(),
				method:         method,
				path:           path,
				responseSize:   metrics.responseSize,
			}
			c.SetResponseWriter(responseWriter)

			// Process the request
			err := next(c)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(c.StatusCode())

			metrics.requestCounter.WithLabelValues(method, path, status).Inc()
			metrics.requestDuration.WithLabelValues(method, path).Observe(duration)

			return err
		}
	})

	// Mount metrics endpoint if enabled
	if config.ExposeMetrics {
		app.GET(config.MetricsPath, func(c flash.Ctx) error {
			// Return Prometheus metrics from the specific registry
			handler := promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{})
			handler.ServeHTTP(c.ResponseWriter(), c.Request())
			return nil
		})
	}
}
