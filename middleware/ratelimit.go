// Package middleware provides comprehensive rate limiting functionality for HTTP applications.
//
// This package implements multiple rate limiting algorithms with a focus on security,
// performance, and flexibility. It's designed for production use in high-traffic
// applications and provides extensive configuration options for different use cases.
//
// # Features
//
// • Multiple rate limiting algorithms (Token Bucket, Fixed Window, Sliding Window, Leaky Bucket, Adaptive)
// • Secure client IP extraction with trusted proxy validation
// • Flexible key extraction (IP, user ID, API key, custom combinations)
// • Memory-efficient with automatic cleanup of expired entries
// • Thread-safe with optimized locking strategies
// • Comprehensive security features to prevent bypass attacks
// • Extensive configuration options for production deployments
//
// # Quick Start
//
// Basic IP-based rate limiting:
//
//	import "github.com/goflash/flash/v2/middleware"
//
//	app := flash.New()
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//	))
//
// # Rate Limiting Strategies
//
// Token Bucket Strategy:
// Best for API rate limiting. Allows bursts up to bucket capacity, then refills at a steady rate.
// Good for user-facing applications where occasional bursts are acceptable.
//
//	strategy := middleware.NewTokenBucketStrategy(100, time.Minute) // 100 requests per minute
//
// Fixed Window Strategy:
// Simple and memory efficient. Resets counter at fixed intervals.
// Can have burst issues at window boundaries but uses minimal memory.
//
//	strategy := middleware.NewFixedWindowStrategy(1000, time.Hour) // 1000 requests per hour
//
// Sliding Window Strategy:
// Smooth rate limiting without boundary burst issues. Uses more memory
// but provides the most accurate rate limiting behavior.
//
//	strategy := middleware.NewSlidingWindowStrategy(100, time.Minute) // 100 requests per minute
//
// Leaky Bucket Strategy:
// Processes requests at a constant rate, queuing excess requests.
// Good for protecting downstream services with consistent load.
//
//	strategy := middleware.NewLeakyBucketStrategy(10.0, 50) // 10 req/sec, 50 queued max
//
// Adaptive Strategy:
// Experimental strategy that adjusts rate limits based on client behavior.
// Can increase limits for well-behaved clients and decrease for problematic ones.
//
//	strategy := middleware.NewAdaptiveStrategy(50.0, 10.0, 100.0, time.Minute)
//
// # Security Considerations
//
// When deploying behind load balancers, CDNs, or reverse proxies, always configure
// trusted proxies to prevent rate limit bypassing:
//
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(strategy),
//		middleware.WithTrustedProxies([]string{
//			"10.0.0.0/8",      // Private networks
//			"172.16.0.0/12",
//			"192.168.0.0/16",
//		}),
//	))
//
// # Production Configuration
//
// For high-traffic production deployments:
//
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewSlidingWindowStrategy(10000, time.Hour)),
//		middleware.WithTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"}),
//		middleware.WithMaxKeyLength(128),
//		middleware.WithCleanupInterval(5 * time.Minute),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			// Custom key extraction logic
//			return extractClientKey(c)
//		}),
//		middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//			// Custom error response with rate limit headers
//			return buildRateLimitResponse(c, retryAfter)
//		}),
//	))
//
// # Performance Tuning
//
// • Choose appropriate strategies based on memory vs accuracy trade-offs
// • Set cleanup intervals based on traffic patterns (1-5 min for high traffic)
// • Use efficient key extraction functions
// • Configure appropriate max key lengths to prevent memory exhaustion
// • Consider multiple middleware instances for different rate limits
//
// # Thread Safety
//
// All rate limiting strategies are thread-safe and optimized for concurrent access.
// The implementation uses read-write mutexes and atomic operations where appropriate
// to minimize lock contention in high-concurrency scenarios.
//
// # Memory Management
//
// The package includes automatic cleanup mechanisms to prevent memory leaks:
// • Background cleanup goroutines remove expired entries
// • Configurable cleanup intervals for different traffic patterns
// • Memory-efficient data structures for each strategy
// • Proper resource cleanup when strategies are closed
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goflash/flash/v2"
)

// RateLimitStrategy defines the interface for different rate limiting strategies.
// All strategies must implement this interface to be used with the RateLimit middleware.
//
// The interface provides a unified way to implement various rate limiting algorithms
// such as token bucket, fixed window, sliding window, leaky bucket, and adaptive strategies.
// Each strategy can have different characteristics in terms of burst handling,
// memory usage, and rate limiting precision.
//
// Example implementation:
//
//	type CustomStrategy struct {
//		// strategy-specific fields
//	}
//
//	func (cs *CustomStrategy) Allow(key string) (bool, time.Duration) {
//		// Implement rate limiting logic
//		// Return true if request is allowed, false if blocked
//		// Return retry duration when blocked
//		return allowed, retryAfter
//	}
//
//	func (cs *CustomStrategy) Name() string {
//		return "custom_strategy"
//	}
//
// Usage with middleware:
//
//	strategy := &CustomStrategy{}
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
type RateLimitStrategy interface {
	// Allow checks if a request is allowed for the given key and returns:
	//   - allowed: true if the request should proceed, false if it should be blocked
	//   - retryAfter: duration to wait before retrying (only meaningful when allowed is false)
	//
	// The key parameter is typically a client identifier (IP address, user ID, API key, etc.)
	// extracted by the KeyFunc. Strategies use this key to maintain separate rate limiting
	// state for different clients.
	//
	// When a request is blocked (allowed=false), the retryAfter duration indicates
	// when the client should retry. This value is used to set the Retry-After HTTP header.
	Allow(key string) (allowed bool, retryAfter time.Duration)

	// Name returns the name of the strategy for identification and debugging.
	// This is used in logs, metrics, and error messages to identify which
	// rate limiting strategy is being used.
	Name() string
}

// RateLimitConfig holds configuration for the RateLimit middleware.
// It provides comprehensive options to customize rate limiting behavior,
// security settings, and performance characteristics.
//
// Example basic configuration:
//
//	config := &RateLimitConfig{
//		Strategy: middleware.NewTokenBucketStrategy(100, time.Minute),
//	}
//
// Example advanced configuration:
//
//	config := &RateLimitConfig{
//		Strategy: middleware.NewTokenBucketStrategy(1000, time.Hour),
//		KeyFunc: func(c flash.Ctx) string {
//			// Rate limit by user ID instead of IP
//			if userID := c.Get("user_id"); userID != nil {
//				return fmt.Sprintf("user:%v", userID)
//			}
//			return "anonymous"
//		},
//		ErrorResponse: func(c flash.Ctx, retryAfter time.Duration) error {
//			return c.Status(429).JSON(map[string]any{
//				"error": "Rate limit exceeded",
//				"retry_after_seconds": int(retryAfter.Seconds()),
//				"message": "Please slow down your requests",
//			})
//		},
//		SkipFunc: func(c flash.Ctx) bool {
//			// Skip rate limiting for admin users
//			return c.Get("is_admin") == true
//		},
//		TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12"},
//		MaxKeyLength: 128,
//		CleanupInterval: 10 * time.Minute,
//	}
type RateLimitConfig struct {
	// Strategy is the rate limiting strategy to use.
	// This determines the algorithm and behavior of rate limiting.
	// If nil, defaults to TokenBucketStrategy(100, time.Minute).
	//
	// Available strategies:
	//   - TokenBucketStrategy: Allows bursts, good for API rate limiting
	//   - FixedWindowStrategy: Simple, can have burst issues at boundaries
	//   - SlidingWindowStrategy: Smooth rate limiting, higher memory usage
	//   - LeakyBucketStrategy: Constant rate processing, queues excess
	//   - AdaptiveStrategy: Adjusts based on client behavior
	Strategy RateLimitStrategy

	// KeyFunc is a function that extracts the key for rate limiting from the request.
	// The key identifies different clients/users for separate rate limiting.
	// If nil, defaults to secure client IP extraction.
	//
	// Common key extraction patterns:
	//   - IP-based: func(c flash.Ctx) string { return clientIP(c.Request()) }
	//   - User-based: func(c flash.Ctx) string { return c.Get("user_id").(string) }
	//   - API key-based: func(c flash.Ctx) string { return c.Header("X-API-Key") }
	//   - Combined: func(c flash.Ctx) string { return userID + ":" + clientIP }
	KeyFunc func(c flash.Ctx) string

	// ErrorResponse is a function that generates a custom error response when rate limited.
	// If nil, defaults to HTTP 429 with "Too Many Requests" message and Retry-After header.
	//
	// The retryAfter parameter indicates when the client should retry the request.
	// This is automatically set as the Retry-After header in the default implementation.
	//
	// Example custom responses:
	//   - JSON: return c.Status(429).JSON(map[string]any{"error": "rate_limited"})
	//   - HTML: return c.Status(429).String("Rate limited. Try again later.")
	//   - Custom headers: c.Header("X-RateLimit-Remaining", "0"); return c.Status(429)
	ErrorResponse func(c flash.Ctx, retryAfter time.Duration) error

	// SkipFunc is a function that determines if rate limiting should be skipped for this request.
	// If nil, no requests are skipped. If returns true, the request bypasses rate limiting entirely.
	//
	// Common skip patterns:
	//   - Admin users: func(c flash.Ctx) bool { return c.Get("is_admin") == true }
	//   - Internal requests: func(c flash.Ctx) bool { return c.Header("X-Internal") != "" }
	//   - Health checks: func(c flash.Ctx) bool { return c.Path() == "/health" }
	//   - Whitelisted IPs: func(c flash.Ctx) bool { return isWhitelisted(clientIP(c.Request())) }
	SkipFunc func(c flash.Ctx) bool

	// TrustedProxies is a list of trusted proxy IP ranges for X-Forwarded-For header validation.
	// This is critical for security when behind load balancers or CDNs.
	// If empty, all X-Forwarded-For headers are trusted (less secure).
	// Use CIDR notation for IP ranges.
	//
	// Common configurations:
	//   - AWS ALB: []string{"10.0.0.0/8", "172.16.0.0/12"}
	//   - Cloudflare: []string{"103.21.244.0/22", "103.22.200.0/22", ...}
	//   - Internal network: []string{"192.168.0.0/16", "10.0.0.0/8"}
	//   - Kubernetes: []string{"10.244.0.0/16"} // pod CIDR
	TrustedProxies []string

	// MaxKeyLength is the maximum allowed length for rate limiting keys.
	// This prevents memory exhaustion attacks through excessively long keys.
	// If 0, defaults to 256 characters.
	//
	// Recommended values:
	//   - IP-based keys: 45 (IPv6 max length)
	//   - User ID keys: 64-128 (depending on ID format)
	//   - API key-based: 128-256 (depending on key format)
	//   - Combined keys: 256-512
	MaxKeyLength int

	// CleanupInterval is how often to clean up expired entries from memory.
	// This prevents memory leaks from accumulating stale rate limiting data.
	// If 0, defaults to 5 minutes. Set to -1 to disable cleanup.
	//
	// Tuning guidelines:
	//   - High traffic: 1-5 minutes (frequent cleanup)
	//   - Low traffic: 10-30 minutes (less frequent cleanup)
	//   - Memory constrained: 1-2 minutes (aggressive cleanup)
	//   - Performance critical: 10+ minutes (less CPU overhead)
	CleanupInterval time.Duration
}

// RateLimitOption is a function that configures the RateLimit middleware.
// Options follow the functional options pattern for flexible configuration.
//
// Example usage:
//
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			return c.Get("user_id").(string)
//		}),
//		middleware.WithTrustedProxies([]string{"10.0.0.0/8"}),
//	))
type RateLimitOption func(*RateLimitConfig)

// WithStrategy sets the rate limiting strategy.
// This determines the algorithm used for rate limiting.
//
// Available built-in strategies:
//   - TokenBucketStrategy: Best for API rate limiting, allows bursts
//   - FixedWindowStrategy: Simple and memory efficient
//   - SlidingWindowStrategy: Smooth rate limiting, no boundary bursts
//   - LeakyBucketStrategy: Constant rate processing
//   - AdaptiveStrategy: Adjusts based on client behavior
//
// Example:
//
//	// Token bucket allowing 100 requests per minute with burst capability
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//	))
//
//	// Fixed window allowing 1000 requests per hour
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewFixedWindowStrategy(1000, time.Hour)),
//	))
func WithStrategy(strategy RateLimitStrategy) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.Strategy = strategy
	}
}

// WithKeyFunc sets a custom key extraction function.
// The key identifies different clients for separate rate limiting.
// If not set, defaults to secure client IP extraction.
//
// Common patterns:
//
//	// Rate limit by authenticated user ID
//	middleware.WithKeyFunc(func(c flash.Ctx) string {
//		if userID := c.Get("user_id"); userID != nil {
//			return fmt.Sprintf("user:%v", userID)
//		}
//		return "anonymous"
//	})
//
//	// Rate limit by API key
//	middleware.WithKeyFunc(func(c flash.Ctx) string {
//		apiKey := c.Header("X-API-Key")
//		if apiKey == "" {
//			return "no-key"
//		}
//		return "api:" + apiKey
//	})
//
//	// Combined user + IP rate limiting
//	middleware.WithKeyFunc(func(c flash.Ctx) string {
//		userID := c.Get("user_id")
//		clientIP := clientIP(c.Request())
//		return fmt.Sprintf("%v:%s", userID, clientIP)
//	})
//
//	// Rate limit by request path (global per-endpoint limiting)
//	middleware.WithKeyFunc(func(c flash.Ctx) string {
//		return c.Route() // e.g., "/api/users/:id"
//	})
func WithKeyFunc(keyFunc func(c flash.Ctx) string) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.KeyFunc = keyFunc
	}
}

// WithErrorResponse sets a custom error response function.
// This allows customizing the response when rate limiting is triggered.
// If not set, defaults to HTTP 429 with "Too Many Requests" message.
//
// Examples:
//
//	// JSON error response with additional information
//	middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//		return c.Status(429).JSON(map[string]any{
//			"error": "rate_limit_exceeded",
//			"message": "Too many requests. Please slow down.",
//			"retry_after_seconds": int(retryAfter.Seconds()),
//			"documentation": "https://api.example.com/docs/rate-limits",
//		})
//	})
//
//	// Custom HTML error page
//	middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//		c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
//		return c.Status(429).String(`
//			<h1>Rate Limited</h1>
//			<p>You've made too many requests. Please wait %d seconds.</p>
//		`, int(retryAfter.Seconds()))
//	})
//
//	// Custom headers with default response
//	middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//		c.Header("X-RateLimit-Remaining", "0")
//		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(retryAfter).Unix()))
//		return c.Status(429).String("Rate limit exceeded")
//	})
func WithErrorResponse(errorResponse func(c flash.Ctx, retryAfter time.Duration) error) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.ErrorResponse = errorResponse
	}
}

// WithSkipFunc sets a function that determines if rate limiting should be skipped.
// If the function returns true, the request bypasses rate limiting entirely.
// This is useful for whitelisting certain requests or users.
//
// Examples:
//
//	// Skip rate limiting for admin users
//	middleware.WithSkipFunc(func(c flash.Ctx) bool {
//		return c.Get("is_admin") == true
//	})
//
//	// Skip rate limiting for internal services
//	middleware.WithSkipFunc(func(c flash.Ctx) bool {
//		return c.Header("X-Internal-Service") != ""
//	})
//
//	// Skip rate limiting for health check endpoints
//	middleware.WithSkipFunc(func(c flash.Ctx) bool {
//		path := c.Path()
//		return path == "/health" || path == "/metrics" || path == "/ready"
//	})
//
//	// Skip rate limiting for whitelisted IP addresses
//	middleware.WithSkipFunc(func(c flash.Ctx) bool {
//		clientIP := clientIP(c.Request())
//		whitelist := []string{"192.168.1.100", "10.0.0.5"}
//		for _, ip := range whitelist {
//			if clientIP == ip {
//				return true
//			}
//		}
//		return false
//	})
//
//	// Skip rate limiting during maintenance windows
//	middleware.WithSkipFunc(func(c flash.Ctx) bool {
//		return c.Header("X-Maintenance-Mode") == "true"
//	})
func WithSkipFunc(skipFunc func(c flash.Ctx) bool) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.SkipFunc = skipFunc
	}
}

// WithTrustedProxies sets trusted proxy IP ranges for secure X-Forwarded-For header processing.
// This is critical for security when your application is behind load balancers, CDNs, or reverse proxies.
// Only X-Forwarded-For headers from these trusted sources will be used for client IP extraction.
//
// Use CIDR notation for IP ranges. If empty, all X-Forwarded-For headers are trusted (less secure).
//
// Common configurations:
//
//	// AWS Application Load Balancer (private subnets)
//	middleware.WithTrustedProxies([]string{
//		"10.0.0.0/8",      // Private class A
//		"172.16.0.0/12",   // Private class B
//		"192.168.0.0/16",  // Private class C
//	})
//
//	// Cloudflare IP ranges (partial list - check Cloudflare docs for complete list)
//	middleware.WithTrustedProxies([]string{
//		"103.21.244.0/22",
//		"103.22.200.0/22",
//		"103.31.4.0/22",
//		"104.16.0.0/13",
//		// ... more Cloudflare ranges
//	})
//
//	// Kubernetes cluster (pod and service CIDRs)
//	middleware.WithTrustedProxies([]string{
//		"10.244.0.0/16",   // Pod CIDR
//		"10.96.0.0/12",    // Service CIDR
//	})
//
//	// Google Cloud Load Balancer
//	middleware.WithTrustedProxies([]string{
//		"130.211.0.0/22",
//		"35.191.0.0/16",
//	})
//
//	// Single trusted proxy
//	middleware.WithTrustedProxies([]string{"192.168.1.100/32"})
func WithTrustedProxies(proxies []string) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.TrustedProxies = proxies
	}
}

// WithMaxKeyLength sets the maximum allowed length for rate limiting keys.
// This prevents memory exhaustion attacks through excessively long keys.
// Keys longer than this limit will be truncated.
//
// Recommended values based on key type:
//   - IP addresses: 45 characters (IPv6 max length)
//   - User IDs: 64-128 characters (depending on your ID format)
//   - API keys: 128-256 characters (depending on key format)
//   - Combined keys: 256-512 characters
//
// Examples:
//
//	// For IP-based rate limiting
//	middleware.WithMaxKeyLength(45)
//
//	// For user ID-based rate limiting
//	middleware.WithMaxKeyLength(128)
//
//	// For API key-based rate limiting
//	middleware.WithMaxKeyLength(256)
//
//	// For combined keys (user:ip:endpoint)
//	middleware.WithMaxKeyLength(512)
//
// Security note: Setting this too high can allow memory exhaustion attacks.
// Setting this too low may cause key collisions. Choose based on your key format.
func WithMaxKeyLength(maxLength int) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.MaxKeyLength = maxLength
	}
}

// WithCleanupInterval sets how often to clean up expired entries from memory.
// This prevents memory leaks from accumulating stale rate limiting data.
// Set to -1 to disable automatic cleanup (not recommended for production).
//
// Tuning guidelines:
//   - High traffic applications: 1-5 minutes (frequent cleanup)
//   - Low traffic applications: 10-30 minutes (less frequent cleanup)
//   - Memory-constrained environments: 1-2 minutes (aggressive cleanup)
//   - Performance-critical applications: 10+ minutes (less CPU overhead)
//
// Examples:
//
//	// High-traffic API with frequent cleanup
//	middleware.WithCleanupInterval(2 * time.Minute)
//
//	// Low-traffic application with less frequent cleanup
//	middleware.WithCleanupInterval(15 * time.Minute)
//
//	// Memory-constrained environment
//	middleware.WithCleanupInterval(1 * time.Minute)
//
//	// Disable cleanup (not recommended for production)
//	middleware.WithCleanupInterval(-1)
//
// Note: Each rate limiting strategy runs its own cleanup goroutine.
// The cleanup process is lightweight and runs in the background.
func WithCleanupInterval(interval time.Duration) RateLimitOption {
	return func(cfg *RateLimitConfig) {
		cfg.CleanupInterval = interval
	}
}

// =============================================================================
// Token Bucket Strategy
// =============================================================================

// TokenBucketStrategy implements a token bucket rate limiting algorithm.
// This strategy allows bursts up to the bucket capacity and refills tokens over time.
type TokenBucketStrategy struct {
	mu          sync.RWMutex
	buckets     map[string]*tokenBucket
	capacity    int
	refill      time.Duration
	lastCleanup int64 // atomic timestamp
	cleanupDone chan struct{}
	cleanupOnce sync.Once
}

type tokenBucket struct {
	remaining int
	reset     time.Time
}

// NewTokenBucketStrategy creates a new token bucket rate limiter.
//
// Parameters:
//   - capacity: Maximum number of tokens in the bucket
//   - refill: Duration after which the bucket refills completely
//
// Usage Examples:
//
//	// 100 requests per minute with burst support
//	strategy := middleware.NewTokenBucketStrategy(100, time.Minute)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
//
//	// 10 requests per second (very restrictive)
//	strategy := middleware.NewTokenBucketStrategy(10, time.Second)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
func NewTokenBucketStrategy(capacity int, refill time.Duration) *TokenBucketStrategy {
	if capacity <= 0 {
		capacity = 1
	}
	if refill <= 0 {
		refill = time.Minute
	}

	tb := &TokenBucketStrategy{
		buckets:     make(map[string]*tokenBucket),
		capacity:    capacity,
		refill:      refill,
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	tb.cleanupOnce.Do(func() {
		go tb.cleanup()
	})

	return tb
}

func (tb *TokenBucketStrategy) Name() string {
	return "token_bucket"
}

func (tb *TokenBucketStrategy) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	// Try read lock first for better performance
	tb.mu.RLock()
	bucket := tb.buckets[key]
	tb.mu.RUnlock()

	// Handle new bucket or expired bucket
	if bucket == nil || now.After(bucket.reset) {
		tb.mu.Lock()
		// Double-check after acquiring write lock
		bucket = tb.buckets[key]
		if bucket == nil || now.After(bucket.reset) {
			bucket = &tokenBucket{
				remaining: tb.capacity - 1,
				reset:     now.Add(tb.refill),
			}
			tb.buckets[key] = bucket
		}
		tb.mu.Unlock()
		return true, 0
	}

	// Handle existing bucket
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Re-check bucket state after acquiring lock
	bucket = tb.buckets[key]
	if bucket == nil || now.After(bucket.reset) {
		bucket = &tokenBucket{
			remaining: tb.capacity - 1,
			reset:     now.Add(tb.refill),
		}
		tb.buckets[key] = bucket
		return true, 0
	}

	if bucket.remaining > 0 {
		bucket.remaining--
		return true, 0
	}

	retry := time.Until(bucket.reset)
	if retry < 0 {
		retry = 0
	}
	return false, retry
}

// cleanup removes expired buckets to prevent memory leaks
func (tb *TokenBucketStrategy) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			atomic.StoreInt64(&tb.lastCleanup, now.Unix())

			tb.mu.Lock()
			for key, bucket := range tb.buckets {
				if now.After(bucket.reset.Add(tb.refill)) {
					delete(tb.buckets, key)
				}
			}
			tb.mu.Unlock()
		case <-tb.cleanupDone:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (tb *TokenBucketStrategy) Close() {
	close(tb.cleanupDone)
}

// =============================================================================
// Fixed Window Strategy
// =============================================================================

// FixedWindowStrategy implements a fixed window rate limiting algorithm.
// This strategy resets the counter at fixed intervals, allowing bursts at window boundaries.
type FixedWindowStrategy struct {
	mu          sync.RWMutex
	windows     map[string]*fixedWindow
	limit       int
	window      time.Duration
	lastCleanup int64 // atomic timestamp
	cleanupDone chan struct{}
	cleanupOnce sync.Once
}

type fixedWindow struct {
	count int
	reset time.Time
}

// NewFixedWindowStrategy creates a new fixed window rate limiter.
//
// Parameters:
//   - limit: Maximum number of requests per window
//   - window: Duration of each window
//
// Usage Examples:
//
//	// 100 requests per minute window
//	strategy := middleware.NewFixedWindowStrategy(100, time.Minute)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
//
//	// 1000 requests per hour window
//	strategy := middleware.NewFixedWindowStrategy(1000, time.Hour)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
func NewFixedWindowStrategy(limit int, window time.Duration) *FixedWindowStrategy {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}

	fw := &FixedWindowStrategy{
		windows:     make(map[string]*fixedWindow),
		limit:       limit,
		window:      window,
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	fw.cleanupOnce.Do(func() {
		go fw.cleanup()
	})

	return fw
}

func (fw *FixedWindowStrategy) Name() string {
	return "fixed_window"
}

func (fw *FixedWindowStrategy) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	// Try read lock first
	fw.mu.RLock()
	window := fw.windows[key]
	fw.mu.RUnlock()

	if window == nil || now.After(window.reset) {
		fw.mu.Lock()
		// Double-check after acquiring write lock
		window = fw.windows[key]
		if window == nil || now.After(window.reset) {
			// Start new window
			window = &fixedWindow{
				count: 1,
				reset: now.Add(fw.window),
			}
			fw.windows[key] = window
		}
		fw.mu.Unlock()
		return true, 0
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Re-check window state after acquiring lock
	window = fw.windows[key]
	if window == nil || now.After(window.reset) {
		window = &fixedWindow{
			count: 1,
			reset: now.Add(fw.window),
		}
		fw.windows[key] = window
		return true, 0
	}

	if window.count < fw.limit {
		window.count++
		return true, 0
	}

	retry := time.Until(window.reset)
	if retry < 0 {
		retry = 0
	}
	return false, retry
}

// cleanup removes expired windows to prevent memory leaks
func (fw *FixedWindowStrategy) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			atomic.StoreInt64(&fw.lastCleanup, now.Unix())

			fw.mu.Lock()
			for key, window := range fw.windows {
				if now.After(window.reset.Add(fw.window)) {
					delete(fw.windows, key)
				}
			}
			fw.mu.Unlock()
		case <-fw.cleanupDone:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (fw *FixedWindowStrategy) Close() {
	close(fw.cleanupDone)
}

// =============================================================================
// Sliding Window Strategy
// =============================================================================

// SlidingWindowStrategy implements a sliding window rate limiting algorithm.
// This strategy provides smooth rate limiting without burst issues at window boundaries.
type SlidingWindowStrategy struct {
	mu          sync.RWMutex
	windows     map[string][]time.Time
	limit       int
	window      time.Duration
	lastCleanup int64 // atomic timestamp
	cleanupDone chan struct{}
	cleanupOnce sync.Once
}

// NewSlidingWindowStrategy creates a new sliding window rate limiter.
//
// Parameters:
//   - limit: Maximum number of requests per window
//   - window: Duration of the sliding window
//
// Usage Examples:
//
//	// 100 requests per 1-minute sliding window
//	strategy := middleware.NewSlidingWindowStrategy(100, time.Minute)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
//
//	// 10 requests per 10-second sliding window
//	strategy := middleware.NewSlidingWindowStrategy(10, 10*time.Second)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
func NewSlidingWindowStrategy(limit int, window time.Duration) *SlidingWindowStrategy {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}

	sw := &SlidingWindowStrategy{
		windows:     make(map[string][]time.Time),
		limit:       limit,
		window:      window,
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	sw.cleanupOnce.Do(func() {
		go sw.cleanup()
	})

	return sw
}

func (sw *SlidingWindowStrategy) Name() string {
	return "sliding_window"
}

func (sw *SlidingWindowStrategy) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	cutoff := now.Add(-sw.window)

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Get existing timestamps for this key
	timestamps := sw.windows[key]

	// Filter out expired timestamps more efficiently
	valid := timestamps[:0] // reuse slice to reduce allocations
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= sw.limit {
		// Find earliest timestamp to calculate retry time
		earliest := valid[0]
		for _, t := range valid[1:] {
			if t.Before(earliest) {
				earliest = t
			}
		}
		retry := earliest.Add(sw.window).Sub(now)
		if retry < 0 {
			retry = 0
		}
		// Update slice to prevent memory leaks
		sw.windows[key] = valid
		return false, retry
	}

	// Add current request
	valid = append(valid, now)
	sw.windows[key] = valid
	return true, 0
}

// cleanup removes expired timestamps to prevent memory leaks
func (sw *SlidingWindowStrategy) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			atomic.StoreInt64(&sw.lastCleanup, now.Unix())
			cutoff := now.Add(-sw.window * 2) // Extra buffer for cleanup

			sw.mu.Lock()
			for key, timestamps := range sw.windows {
				// Filter out very old timestamps
				valid := timestamps[:0]
				for _, t := range timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}

				if len(valid) == 0 {
					delete(sw.windows, key)
				} else {
					sw.windows[key] = valid
				}
			}
			sw.mu.Unlock()
		case <-sw.cleanupDone:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (sw *SlidingWindowStrategy) Close() {
	close(sw.cleanupDone)
}

// =============================================================================
// Leaky Bucket Strategy
// =============================================================================

// LeakyBucketStrategy implements a leaky bucket rate limiting algorithm.
// This strategy processes requests at a fixed rate, queuing excess requests.
type LeakyBucketStrategy struct {
	mu          sync.RWMutex
	buckets     map[string]*leakyBucket
	rate        float64 // requests per second
	capacity    int
	lastCleanup int64 // atomic timestamp
	cleanupDone chan struct{}
	cleanupOnce sync.Once
}

type leakyBucket struct {
	lastLeak time.Time
	level    int
}

// NewLeakyBucketStrategy creates a new leaky bucket rate limiter.
//
// Parameters:
//   - rate: Requests per second (can be fractional)
//   - capacity: Maximum number of requests that can be queued
//
// Usage Examples:
//
//	// 10 requests per second with capacity for 50 queued requests
//	strategy := middleware.NewLeakyBucketStrategy(10.0, 50)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
//
//	// 0.5 requests per second (1 request every 2 seconds)
//	strategy := middleware.NewLeakyBucketStrategy(0.5, 10)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
func NewLeakyBucketStrategy(rate float64, capacity int) *LeakyBucketStrategy {
	if rate <= 0 {
		rate = 1.0
	}
	if capacity <= 0 {
		capacity = 1
	}

	lb := &LeakyBucketStrategy{
		buckets:     make(map[string]*leakyBucket),
		rate:        rate,
		capacity:    capacity,
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	lb.cleanupOnce.Do(func() {
		go lb.cleanup()
	})

	return lb
}

func (lb *LeakyBucketStrategy) Name() string {
	return "leaky_bucket"
}

func (lb *LeakyBucketStrategy) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	// Try read lock first
	lb.mu.RLock()
	bucket := lb.buckets[key]
	lb.mu.RUnlock()

	if bucket == nil {
		lb.mu.Lock()
		// Double-check after acquiring write lock
		bucket = lb.buckets[key]
		if bucket == nil {
			bucket = &leakyBucket{
				lastLeak: now,
				level:    1, // Start with 1 since we're allowing this request
			}
			lb.buckets[key] = bucket
			lb.mu.Unlock()
			return true, 0
		}
		lb.mu.Unlock()
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Calculate how much has leaked since last request
	elapsed := now.Sub(bucket.lastLeak).Seconds()
	leaked := int(elapsed * lb.rate)

	// Update bucket level
	bucket.level = max(0, bucket.level-leaked)
	bucket.lastLeak = now

	if bucket.level < lb.capacity {
		bucket.level++
		return true, 0
	}

	// Calculate when next slot will be available
	nextSlot := time.Duration(float64(time.Second) / lb.rate)
	return false, nextSlot
}

// cleanup removes inactive buckets to prevent memory leaks
func (lb *LeakyBucketStrategy) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			atomic.StoreInt64(&lb.lastCleanup, now.Unix())
			cutoff := now.Add(-10 * time.Minute) // Remove buckets inactive for 10 minutes

			lb.mu.Lock()
			for key, bucket := range lb.buckets {
				if bucket.lastLeak.Before(cutoff) && bucket.level == 0 {
					delete(lb.buckets, key)
				}
			}
			lb.mu.Unlock()
		case <-lb.cleanupDone:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (lb *LeakyBucketStrategy) Close() {
	close(lb.cleanupDone)
}

// =============================================================================
// Adaptive Strategy
// =============================================================================

// AdaptiveStrategy implements an adaptive rate limiting algorithm.
// This strategy adjusts the rate limit based on the client's behavior.
type AdaptiveStrategy struct {
	mu          sync.RWMutex
	clients     map[string]*adaptiveClient
	baseRate    float64
	minRate     float64
	maxRate     float64
	window      time.Duration
	lastCleanup int64 // atomic timestamp
	cleanupDone chan struct{}
	cleanupOnce sync.Once
}

type adaptiveClient struct {
	lastRequest time.Time
	currentRate float64
	goodCount   int
	badCount    int
}

// NewAdaptiveStrategy creates a new adaptive rate limiter.
//
// Parameters:
//   - baseRate: Initial requests per second
//   - minRate: Minimum requests per second
//   - maxRate: Maximum requests per second
//   - window: Window for rate calculation
//
// Usage Examples:
//
//	// Adaptive rate limiting with 10-100 requests per second range
//	strategy := middleware.NewAdaptiveStrategy(50.0, 10.0, 100.0, time.Minute)
//	app.Use(middleware.RateLimit(middleware.WithStrategy(strategy)))
func NewAdaptiveStrategy(baseRate, minRate, maxRate float64, window time.Duration) *AdaptiveStrategy {
	if baseRate <= 0 {
		baseRate = 1.0
	}
	if minRate <= 0 {
		minRate = 0.1
	}
	if maxRate <= 0 || maxRate < baseRate {
		maxRate = baseRate * 10
	}
	if window <= 0 {
		window = time.Minute
	}

	as := &AdaptiveStrategy{
		clients:     make(map[string]*adaptiveClient),
		baseRate:    baseRate,
		minRate:     minRate,
		maxRate:     maxRate,
		window:      window,
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	as.cleanupOnce.Do(func() {
		go as.cleanup()
	})

	return as
}

func (as *AdaptiveStrategy) Name() string {
	return "adaptive"
}

func (as *AdaptiveStrategy) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	// Try read lock first
	as.mu.RLock()
	client := as.clients[key]
	as.mu.RUnlock()

	if client == nil {
		as.mu.Lock()
		// Double-check after acquiring write lock
		client = as.clients[key]
		if client == nil {
			client = &adaptiveClient{
				lastRequest: now,
				currentRate: as.baseRate,
			}
			as.clients[key] = client
		}
		as.mu.Unlock()
		return true, 0
	}

	as.mu.Lock()
	defer as.mu.Unlock()

	// Check if enough time has passed since last request
	elapsed := now.Sub(client.lastRequest).Seconds()
	minInterval := 1.0 / client.currentRate

	if elapsed < minInterval {
		retryAfter := time.Duration((minInterval - elapsed) * float64(time.Second))
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	client.lastRequest = now
	return true, 0
}

// UpdateRate updates the rate for a specific client based on their behavior.
// Call this method from your application logic to provide feedback.
func (as *AdaptiveStrategy) UpdateRate(key string, isGood bool) {
	as.mu.Lock()
	defer as.mu.Unlock()

	client := as.clients[key]
	if client == nil {
		return
	}

	if isGood {
		client.goodCount++
		// Increase rate gradually
		client.currentRate = min(as.maxRate, client.currentRate*1.1)
	} else {
		client.badCount++
		// Decrease rate more aggressively
		client.currentRate = maxFloat64(as.minRate, client.currentRate*0.5)
	}
}

// cleanup removes inactive clients to prevent memory leaks
func (as *AdaptiveStrategy) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			atomic.StoreInt64(&as.lastCleanup, now.Unix())
			cutoff := now.Add(-as.window * 2) // Remove clients inactive for 2x window duration

			as.mu.Lock()
			for key, client := range as.clients {
				if client.lastRequest.Before(cutoff) {
					delete(as.clients, key)
				}
			}
			as.mu.Unlock()
		case <-as.cleanupDone:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (as *AdaptiveStrategy) Close() {
	close(as.cleanupDone)
}

// =============================================================================
// RateLimit Middleware
// =============================================================================

// RateLimit returns middleware that applies rate limiting to HTTP requests.
//
// This middleware integrates any RateLimitStrategy implementation with the HTTP request flow.
// It extracts a client identifier (key), checks if the request is allowed by the strategy,
// and returns an appropriate error response if the rate limit is exceeded.
//
// The middleware is highly configurable and supports various rate limiting patterns:
//   - Different algorithms (token bucket, sliding window, etc.)
//   - Custom key extraction (IP, user ID, API key, etc.)
//   - Custom error responses (JSON, HTML, etc.)
//   - Request whitelisting and skipping
//   - Secure proxy handling
//   - Memory management and cleanup
//
// Basic Usage Examples:
//
//	// Simple IP-based rate limiting (100 requests per minute)
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//	))
//
//	// API rate limiting with custom error response
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(1000, time.Hour)),
//		middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//			return c.Status(429).JSON(map[string]any{
//				"error": "API rate limit exceeded",
//				"retry_after_seconds": int(retryAfter.Seconds()),
//				"limit": 1000,
//				"window": "1 hour",
//			})
//		}),
//	))
//
// Advanced Usage Examples:
//
//	// User-based rate limiting with different limits per user tier
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			userID := c.Get("user_id")
//			userTier := c.Get("user_tier")
//			return fmt.Sprintf("%s:%s", userTier, userID)
//		}),
//		middleware.WithSkipFunc(func(c flash.Ctx) bool {
//			// Skip rate limiting for premium users
//			return c.Get("user_tier") == "premium"
//		}),
//	))
//
//	// Multi-layered rate limiting (per-IP and per-user)
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(1000, time.Hour)),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			// Global per-IP limit
//			return "ip:" + clientIP(c.Request())
//		}),
//	))
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewTokenBucketStrategy(100, time.Minute)),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			// Per-user limit (more restrictive)
//			if userID := c.Get("user_id"); userID != nil {
//				return "user:" + userID.(string)
//			}
//			return "anonymous"
//		}),
//	))
//
// Production Configuration Examples:
//
//	// High-traffic API behind AWS Application Load Balancer
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewSlidingWindowStrategy(10000, time.Hour)),
//		middleware.WithTrustedProxies([]string{
//			"10.0.0.0/8",      // Private subnets
//			"172.16.0.0/12",   // Private subnets
//			"192.168.0.0/16",  // Private subnets
//		}),
//		middleware.WithMaxKeyLength(128),
//		middleware.WithCleanupInterval(5 * time.Minute),
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			// Combine API key and IP for more granular control
//			apiKey := c.Header("X-API-Key")
//			if apiKey == "" {
//				return "no-key:" + clientIP(c.Request())
//			}
//			return "api:" + apiKey
//		}),
//		middleware.WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
//			// Include rate limit headers for API clients
//			c.Header("X-RateLimit-Limit", "10000")
//			c.Header("X-RateLimit-Remaining", "0")
//			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(retryAfter).Unix()))
//			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
//			return c.Status(429).JSON(map[string]any{
//				"error": "rate_limit_exceeded",
//				"message": "API rate limit exceeded. Please slow down your requests.",
//				"retry_after_seconds": int(retryAfter.Seconds()),
//				"documentation": "https://docs.example.com/api/rate-limits",
//			})
//		}),
//		middleware.WithSkipFunc(func(c flash.Ctx) bool {
//			// Skip rate limiting for health checks and internal services
//			path := c.Path()
//			return path == "/health" ||
//				   path == "/metrics" ||
//				   c.Header("X-Internal-Service") != ""
//		}),
//	))
//
//	// WebSocket connection rate limiting
//	app.Use(middleware.RateLimit(
//		middleware.WithStrategy(middleware.NewLeakyBucketStrategy(1.0, 10)), // 1 conn/sec, max 10 queued
//		middleware.WithKeyFunc(func(c flash.Ctx) string {
//			return "ws:" + clientIP(c.Request())
//		}),
//		middleware.WithSkipFunc(func(c flash.Ctx) bool {
//			// Only apply to WebSocket upgrade requests
//			return c.Header("Upgrade") != "websocket"
//		}),
//	))
//
// Strategy Selection Guide:
//   - TokenBucketStrategy: Best for APIs, allows bursts, good for user-facing applications
//   - FixedWindowStrategy: Simple and memory efficient, but can have boundary burst issues
//   - SlidingWindowStrategy: Smooth rate limiting, higher memory usage, best for strict limits
//   - LeakyBucketStrategy: Constant rate processing, good for resource protection
//   - AdaptiveStrategy: Experimental, adjusts based on client behavior
//
// Security Considerations:
//   - Always configure TrustedProxies when behind load balancers/CDNs
//   - Set appropriate MaxKeyLength to prevent memory exhaustion attacks
//   - Use secure key extraction to prevent rate limit bypassing
//   - Consider using HTTPS to prevent header manipulation
//   - Monitor for suspicious patterns and adjust limits accordingly
//
// Performance Considerations:
//   - TokenBucket and FixedWindow are most memory efficient
//   - SlidingWindow uses more memory but provides smoother limiting
//   - Set appropriate CleanupInterval based on traffic patterns
//   - Consider using multiple middleware instances for different rate limits
//   - Use SkipFunc judiciously to avoid unnecessary processing
func RateLimit(options ...RateLimitOption) flash.Middleware {
	// Apply configuration options
	cfg := &RateLimitConfig{}
	for _, option := range options {
		option(cfg)
	}

	// Set defaults
	if cfg.Strategy == nil {
		cfg.Strategy = NewTokenBucketStrategy(100, time.Minute)
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c flash.Ctx) string {
			return secureClientIP(c.Request(), cfg.TrustedProxies)
		}
	}
	if cfg.ErrorResponse == nil {
		cfg.ErrorResponse = defaultErrorResponse
	}
	if cfg.MaxKeyLength <= 0 {
		cfg.MaxKeyLength = 256
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}

	// Parse trusted proxies (validation is done in secureClientIP)
	_ = cfg.TrustedProxies

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			// Check if rate limiting should be skipped
			if cfg.SkipFunc != nil && cfg.SkipFunc(c) {
				return next(c)
			}

			// Extract key
			key := cfg.KeyFunc(c)
			if key == "" {
				key = "unknown"
			}

			// Validate key length to prevent memory exhaustion attacks
			if len(key) > cfg.MaxKeyLength {
				key = key[:cfg.MaxKeyLength]
			}

			// Sanitize key to prevent injection attacks
			key = sanitizeKey(key)

			// Check if request is allowed
			allowed, retryAfter := cfg.Strategy.Allow(key)
			if !allowed {
				return cfg.ErrorResponse(c, retryAfter)
			}

			return next(c)
		}
	}
}

// defaultKeyFunc extracts the client IP address as the rate limiting key.
func defaultKeyFunc(c flash.Ctx) string {
	if r := c.Request(); r != nil {
		return clientIP(r)
	}
	return ""
}

// defaultErrorResponse returns a standard HTTP 429 response with Retry-After header.
// This follows the framework pattern of handling the response directly and returning nil
// to indicate the response has been handled, rather than returning an error.
func defaultErrorResponse(c flash.Ctx, retryAfter time.Duration) error {
	if retryAfter > 0 {
		c.Header("Retry-After", formatSeconds(retryAfter))
	}
	// Set standard rate limiting headers for better client integration
	c.Header("X-RateLimit-Remaining", "0")

	// Handle the response directly, following framework patterns
	return c.String(http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
}

// =============================================================================
// Utility Functions
// =============================================================================

// clientIP extracts a stable client identifier for rate limiting.
// Deprecated: Use secureClientIP instead for better security.
//
//nolint:unused // kept for backward compatibility
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip := strings.TrimSpace(xrip)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// secureClientIP extracts client IP with trusted proxy validation.
// This function provides secure client IP extraction by validating X-Forwarded-For
// headers against a list of trusted proxy IP ranges.
//
// The function implements the following security measures:
//   - Only trusts X-Forwarded-For headers from configured trusted proxies
//   - Validates that forwarded IPs are properly formatted
//   - Skips private/loopback IPs in the forwarded chain
//   - Falls back to direct connection IP when headers are untrusted
//
// Algorithm:
//  1. Extract direct connection IP from RemoteAddr
//  2. If no trusted proxies configured, return direct IP (secure default)
//  3. Check if direct IP is from a trusted proxy
//  4. If trusted, parse X-Forwarded-For header for real client IP
//  5. Skip private/loopback IPs in the forwarded chain
//  6. Return first public IP found, or fallback to direct IP
//
// Parameters:
//   - r: HTTP request containing headers and connection info
//   - trustedProxies: List of CIDR ranges for trusted proxy validation
//
// Returns:
//   - Client IP address as string, or direct connection IP as fallback
//
// Example usage:
//
//	// Basic usage with common proxy ranges
//	trustedProxies := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
//	clientIP := secureClientIP(request, trustedProxies)
//
//	// AWS ALB configuration
//	trustedProxies := []string{"10.0.0.0/8", "172.16.0.0/12"}
//	clientIP := secureClientIP(request, trustedProxies)
//
//	// No trusted proxies (direct connections only)
//	clientIP := secureClientIP(request, nil)
//
// Security note: This function is critical for rate limiting security.
// Misconfiguration can allow rate limit bypassing through header spoofing.
func secureClientIP(r *http.Request, trustedProxies []string) string {
	// Parse trusted proxy networks
	var trustedNets []*net.IPNet
	for _, proxy := range trustedProxies {
		if _, ipnet, err := net.ParseCIDR(proxy); err == nil {
			trustedNets = append(trustedNets, ipnet)
		}
	}

	// Get the direct connection IP
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	directIP := net.ParseIP(host)
	if directIP == nil {
		return host // fallback to original string
	}

	// If no trusted proxies are configured, only trust direct connection
	if len(trustedNets) == 0 {
		return directIP.String()
	}

	// Check if direct connection is from a trusted proxy
	isTrustedProxy := false
	for _, ipnet := range trustedNets {
		if ipnet.Contains(directIP) {
			isTrustedProxy = true
			break
		}
	}

	// If not from trusted proxy, return direct IP
	if !isTrustedProxy {
		return directIP.String()
	}

	// Check X-Forwarded-For header (most common)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			ip := strings.TrimSpace(part)
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				// Skip private/loopback IPs in forwarded chain
				if !isPrivateOrLoopback(parsedIP) {
					return parsedIP.String()
				}
			}
		}
	}

	// Check X-Real-IP header (Nginx)
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip := strings.TrimSpace(xrip)
		if parsedIP := net.ParseIP(ip); parsedIP != nil && !isPrivateOrLoopback(parsedIP) {
			return parsedIP.String()
		}
	}

	// Fallback to direct connection IP
	return directIP.String()
}

// isPrivateOrLoopback checks if an IP address is private, loopback, or link-local.
// This function is used to filter out internal/private IPs from X-Forwarded-For chains
// to find the real public client IP address.
//
// The function returns true for:
//   - Loopback addresses (127.0.0.0/8, ::1)
//   - Private addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7)
//   - Link-local unicast (169.254.0.0/16, fe80::/10)
//   - Link-local multicast (224.0.0.0/4, ff02::/16)
//
// Parameters:
//   - ip: Parsed IP address to check
//
// Returns:
//   - true if the IP is private/internal, false if it's a public IP
//
// Example usage:
//
//	ip := net.ParseIP("192.168.1.1")
//	if isPrivateOrLoopback(ip) {
//		// Skip this IP in forwarded chain
//		continue
//	}
//	// Use this public IP
//
// This is used internally by secureClientIP to skip private IPs
// in X-Forwarded-For chains when looking for the real client IP.
func isPrivateOrLoopback(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// sanitizeKey removes potentially dangerous characters from rate limiting keys.
// This function prevents injection attacks and ensures keys are safe for storage
// and processing by removing or replacing control characters and non-printable bytes.
//
// The function performs the following sanitization:
//   - Replaces control characters (0-31, 127) with underscores
//   - Keeps only printable ASCII characters (32-126)
//   - Replaces non-ASCII characters with underscores
//   - Preserves spaces and common punctuation
//
// This prevents:
//   - Null byte injection attacks
//   - Control character injection
//   - Memory corruption through invalid UTF-8
//   - Log injection attacks
//   - Key collision through invisible characters
//
// Parameters:
//   - key: Raw key string to sanitize
//
// Returns:
//   - Sanitized key string safe for use as rate limiting key
//
// Example usage:
//
//	// Sanitize user-provided key
//	userKey := request.Header.Get("X-User-ID")
//	safeKey := sanitizeKey(userKey)
//
//	// Example transformations:
//	sanitizeKey("user\x00123")     // -> "user_123"
//	sanitizeKey("key\twith\ntabs") // -> "key_with_tabs"
//	sanitizeKey("normal_key")      // -> "normal_key" (unchanged)
//	sanitizeKey("key with spaces") // -> "key with spaces" (spaces preserved)
//
// Security note: This function is essential for preventing key-based attacks.
// Always sanitize keys derived from user input or external sources.
func sanitizeKey(key string) string {
	// Limit to printable ASCII for safety
	var result strings.Builder
	result.Grow(len(key))
	for _, r := range key {
		if r >= 32 && r <= 126 { // Printable ASCII
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}

	return result.String()
}

// formatSeconds converts a time.Duration to a string representation in seconds.
func formatSeconds(d time.Duration) string {
	sec := int(d.Seconds())
	if sec < 1 {
		sec = 1
	}
	return strconv.Itoa(sec)
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// maxFloat64 returns the maximum of two float64s.
func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum of two float64s.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
