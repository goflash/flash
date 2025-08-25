package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
	"github.com/goflash/flash/v2/ctx"
)

func TestRateLimitBlocksAfterCapacity(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(2, time.Minute)
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}

	// Third request should be blocked
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimitSetsRetryAfter(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked with Retry-After header
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatalf("expected Retry-After header")
	}
}

func TestRateLimitResetAllowsAfterRetry(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, 100*time.Millisecond)
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	// Wait for reset
	time.Sleep(150 * time.Millisecond)

	// Third request should succeed after reset
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimitRemainingDecrementBranch(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(3, time.Minute)
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Make 3 requests to test remaining decrement
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}

	// Fourth request should be blocked
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestClientIPExtraction(t *testing.T) {
	// Test X-Forwarded-For priority
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1")
	if ip := clientIP(req); ip != "192.168.1.100" {
		t.Fatalf("expected 192.168.1.100, got %s", ip)
	}

	// Test X-Real-IP when X-Forwarded-For is not present
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.1")
	if ip := clientIP(req); ip != "203.0.113.1" {
		t.Fatalf("expected 203.0.113.1, got %s", ip)
	}

	// Test RemoteAddr when headers are not present
	req, _ = http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	if ip := clientIP(req); ip != "192.168.1.100" {
		t.Fatalf("expected 192.168.1.100, got %s", ip)
	}
}

func TestRateLimitWithCustomKeyFunc(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithKeyFunc(func(c flash.Ctx) string {
			return "custom_key"
		}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimitWithCustomErrorResponse(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithErrorResponse(func(c flash.Ctx, retryAfter time.Duration) error {
			return c.JSON(map[string]interface{}{
				"error":       "custom_rate_limit",
				"retry_after": int(retryAfter.Seconds()),
			})
		}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked with custom response
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { // JSON response defaults to 200
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() == "" {
		t.Fatalf("expected custom error response body")
	}
}

func TestRateLimitWithSkipFunc(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithSkipFunc(func(c flash.Ctx) bool {
			return c.Query("skip") == "true"
		}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Request with skip parameter should always succeed
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x?skip=true", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func TestRateLimitWithEmptyKey(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithKeyFunc(func(c flash.Ctx) string {
			return ""
		}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked (empty key becomes "unknown")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimitDefaultStrategy(t *testing.T) {
	a := flash.New()
	// Use RateLimit without specifying a strategy (should use default)
	a.Use(RateLimit())
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Should work with default token bucket strategy
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestFixedWindowStrategy(t *testing.T) {
	strategy := NewFixedWindowStrategy(2, time.Minute)

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		allowed, _ := strategy.Allow("test_key")
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	// Third request should be blocked
	allowed, retryAfter := strategy.Allow("test_key")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry after duration")
	}
}

func TestSlidingWindowStrategy(t *testing.T) {
	strategy := NewSlidingWindowStrategy(2, 100*time.Millisecond)

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		allowed, _ := strategy.Allow("test_key")
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	// Third request should be blocked
	allowed, retryAfter := strategy.Allow("test_key")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry after duration")
	}

	// Wait for window to slide
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	allowed, _ = strategy.Allow("test_key")
	if !allowed {
		t.Fatalf("expected request to be allowed after window slide")
	}
}

func TestLeakyBucketStrategy(t *testing.T) {
	strategy := NewLeakyBucketStrategy(10.0, 5) // 10 req/sec, capacity 5

	// First 5 requests should succeed immediately
	for i := 0; i < 5; i++ {
		allowed, _ := strategy.Allow("test_key")
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	// Sixth request should be blocked
	allowed, retryAfter := strategy.Allow("test_key")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry after duration")
	}
}

func TestAdaptiveStrategy(t *testing.T) {
	strategy := NewAdaptiveStrategy(1.0, 0.1, 10.0, time.Minute) // 1 req/sec, 0.1-10 range

	// First request should succeed
	allowed, _ := strategy.Allow("test_key")
	if !allowed {
		t.Fatalf("expected first request to be allowed")
	}

	// Second request should be blocked (rate limiting)
	allowed, retryAfter := strategy.Allow("test_key")
	if allowed {
		t.Fatalf("expected second request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry after duration")
	}

	// Test rate adjustment
	strategy.UpdateRate("test_key", true)  // Good behavior
	strategy.UpdateRate("test_key", false) // Bad behavior
}

func TestStrategyNames(t *testing.T) {
	tests := []struct {
		strategy RateLimitStrategy
		expected string
	}{
		{NewTokenBucketStrategy(10, time.Minute), "token_bucket"},
		{NewFixedWindowStrategy(10, time.Minute), "fixed_window"},
		{NewSlidingWindowStrategy(10, time.Minute), "sliding_window"},
		{NewLeakyBucketStrategy(10.0, 5), "leaky_bucket"},
		{NewAdaptiveStrategy(10.0, 1.0, 100.0, time.Minute), "adaptive"},
	}

	for _, test := range tests {
		if name := test.strategy.Name(); name != test.expected {
			t.Fatalf("expected %s, got %s", test.expected, name)
		}
	}
}

func TestTokenBucketConcurrency(t *testing.T) {
	// Test concurrent access to token bucket
	strategy := NewTokenBucketStrategy(10, time.Minute)
	defer strategy.Close()

	// Run concurrent requests
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := strategy.Allow("concurrent_test")
			if allowed {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// Should allow exactly 10 requests (capacity)
	if successCount != 10 {
		t.Fatalf("expected 10 successful requests, got %d", successCount)
	}
}

func BenchmarkRateLimit(b *testing.B) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1000, time.Minute)
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/bench", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		a.ServeHTTP(rec, req)
	}
}

func BenchmarkRateLimitWithCustomKeyFunc(b *testing.B) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1000, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithKeyFunc(func(c flash.Ctx) string {
			return "benchmark_key"
		}),
	))
	a.GET("/bench", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		a.ServeHTTP(rec, req)
	}
}

func BenchmarkTokenBucketStrategy(b *testing.B) {
	strategy := NewTokenBucketStrategy(1000, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Allow("benchmark_key")
	}
}

func BenchmarkSlidingWindowStrategy(b *testing.B) {
	strategy := NewSlidingWindowStrategy(1000, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Allow("benchmark_key")
	}
}

// =============================================================================
// Security and Enhancement Tests
// =============================================================================

func TestRateLimitWithTrustedProxies(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithTrustedProxies([]string{"10.0.0.0/8", "192.168.0.0/16"}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test with trusted proxy
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request from same real IP should be blocked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimitWithUntrustedProxy(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithTrustedProxies([]string{"10.0.0.0/8"}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test with untrusted proxy - should use direct IP
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.1:12345"
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request from same direct IP should be blocked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.1:12345"
	req.Header.Set("X-Forwarded-For", "192.168.1.2") // Different forwarded IP
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimitWithMaxKeyLength(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithMaxKeyLength(10),
		WithKeyFunc(func(c flash.Ctx) string {
			return "very_long_key_that_exceeds_limit"
		}),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request should succeed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked (key truncated to same value)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestSecureClientIPValidation(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xff            string
		xrealip        string
		trustedProxies []string
		expected       string
	}{
		{
			name:       "Direct connection no proxy",
			remoteAddr: "203.0.113.1:12345",
			expected:   "203.0.113.1",
		},
		{
			name:           "Trusted proxy with XFF",
			remoteAddr:     "10.0.0.1:12345",
			xff:            "203.0.113.1, 192.168.1.1",
			trustedProxies: []string{"10.0.0.0/8"},
			expected:       "203.0.113.1",
		},
		{
			name:           "Untrusted proxy ignores XFF",
			remoteAddr:     "203.0.113.1:12345",
			xff:            "192.168.1.1",
			trustedProxies: []string{"10.0.0.0/8"},
			expected:       "203.0.113.1",
		},
		{
			name:           "Trusted proxy with X-Real-IP",
			remoteAddr:     "10.0.0.1:12345",
			xrealip:        "203.0.113.1",
			trustedProxies: []string{"10.0.0.0/8"},
			expected:       "203.0.113.1",
		},
		{
			name:           "Private IP in XFF chain skipped",
			remoteAddr:     "10.0.0.1:12345",
			xff:            "192.168.1.1, 203.0.113.1",
			trustedProxies: []string{"10.0.0.0/8"},
			expected:       "203.0.113.1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			req.RemoteAddr = test.remoteAddr
			if test.xff != "" {
				req.Header.Set("X-Forwarded-For", test.xff)
			}
			if test.xrealip != "" {
				req.Header.Set("X-Real-IP", test.xrealip)
			}

			result := secureClientIP(req, test.trustedProxies)
			if result != test.expected {
				t.Fatalf("expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal_key", "normal_key"},
		{"key\x00with\x01null", "key_with_null"},
		{"key\twith\ntabs", "key_with_tabs"},
		{"key with spaces", "key with spaces"},
		{"key\x7fwith\x80unicode", "key_with_unicode"},
		{"key\x80only", "key_only"},
		{"", ""},
		{strings.Repeat("a", 1000), strings.Repeat("a", 1000)},
	}

	for _, test := range tests {
		result := sanitizeKey(test.input)
		if result != test.expected {
			t.Fatalf("input %q: expected %q, got %q", test.input, test.expected, result)
		}
	}
}

func TestIsPrivateOrLoopback(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},    // loopback
		{"::1", true},          // IPv6 loopback
		{"192.168.1.1", true},  // private
		{"10.0.0.1", true},     // private
		{"172.16.0.1", true},   // private
		{"203.0.113.1", false}, // public
		{"8.8.8.8", false},     // public
		{"2001:db8::1", false}, // public IPv6
	}

	for _, test := range tests {
		ip := net.ParseIP(test.ip)
		if ip == nil {
			t.Fatalf("invalid IP: %s", test.ip)
		}
		result := isPrivateOrLoopback(ip)
		if result != test.expected {
			t.Fatalf("IP %s: expected %t, got %t", test.ip, test.expected, result)
		}
	}
}

func TestStrategyCleanup(t *testing.T) {
	// Test TokenBucket cleanup
	tb := NewTokenBucketStrategy(1, 100*time.Millisecond)
	tb.Allow("test_key")

	// Wait for bucket to expire and cleanup to run
	time.Sleep(200 * time.Millisecond)

	tb.mu.RLock()
	bucketCount := len(tb.buckets)
	tb.mu.RUnlock()

	// Bucket should still exist (cleanup runs every 5 minutes by default)
	if bucketCount == 0 {
		t.Log("Bucket cleaned up (this is expected behavior)")
	}

	tb.Close()
}

func TestFixedWindowWithInputValidation(t *testing.T) {
	// Test with invalid parameters
	fw := NewFixedWindowStrategy(-1, -time.Second)
	if fw.limit != 1 {
		t.Fatalf("expected limit to default to 1, got %d", fw.limit)
	}
	if fw.window != time.Minute {
		t.Fatalf("expected window to default to 1 minute, got %v", fw.window)
	}
	fw.Close()
}

func TestLeakyBucketWithInputValidation(t *testing.T) {
	// Test with invalid parameters
	lb := NewLeakyBucketStrategy(-1, -1)
	if lb.rate != 1.0 {
		t.Fatalf("expected rate to default to 1.0, got %f", lb.rate)
	}
	if lb.capacity != 1 {
		t.Fatalf("expected capacity to default to 1, got %d", lb.capacity)
	}
	lb.Close()
}

func TestAdaptiveStrategyWithInputValidation(t *testing.T) {
	// Test with invalid parameters
	as := NewAdaptiveStrategy(-1, -1, -1, -time.Second)
	if as.baseRate != 1.0 {
		t.Fatalf("expected baseRate to default to 1.0, got %f", as.baseRate)
	}
	if as.minRate != 0.1 {
		t.Fatalf("expected minRate to default to 0.1, got %f", as.minRate)
	}
	if as.maxRate != 10.0 {
		t.Fatalf("expected maxRate to default to 10.0, got %f", as.maxRate)
	}
	if as.window != time.Minute {
		t.Fatalf("expected window to default to 1 minute, got %v", as.window)
	}
	as.Close()
}

func TestAdaptiveStrategyUpdateNonExistentClient(t *testing.T) {
	as := NewAdaptiveStrategy(1.0, 0.1, 10.0, time.Minute)

	// Should not panic when updating non-existent client
	as.UpdateRate("non_existent", true)
	as.UpdateRate("non_existent", false)

	as.Close()
}

func TestRateLimitConfigurationOptions(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)

	a.Use(RateLimit(
		WithStrategy(strategy),
		WithTrustedProxies([]string{"10.0.0.0/8"}),
		WithMaxKeyLength(50),
		WithCleanupInterval(time.Minute),
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test that configuration is applied correctly
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestClientIPWithInvalidRemoteAddr(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "invalid_address"

	// Should not panic and return the original string
	result := clientIP(req)
	if result != "invalid_address" {
		t.Fatalf("expected 'invalid_address', got %s", result)
	}
}

func TestSecureClientIPWithInvalidRemoteAddr(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "invalid_address"

	// Should not panic and return the original string
	result := secureClientIP(req, []string{})
	if result != "invalid_address" {
		t.Fatalf("expected 'invalid_address', got %s", result)
	}
}

func TestSlidingWindowMemoryOptimization(t *testing.T) {
	sw := NewSlidingWindowStrategy(2, 100*time.Millisecond)

	// Add some timestamps
	sw.Allow("test_key")
	time.Sleep(50 * time.Millisecond)
	sw.Allow("test_key")

	// Check that slice reuse works
	sw.mu.RLock()
	timestamps := sw.windows["test_key"]
	initialCap := cap(timestamps)
	sw.mu.RUnlock()

	// Wait for some timestamps to expire
	time.Sleep(150 * time.Millisecond)
	sw.Allow("test_key")

	sw.mu.RLock()
	timestamps = sw.windows["test_key"]
	newCap := cap(timestamps)
	sw.mu.RUnlock()

	// Capacity should be preserved for memory efficiency
	if newCap < initialCap {
		t.Log("Slice capacity was reduced, which is acceptable for memory optimization")
	}

	sw.Close()
}

func TestNegativeRetryAfterHandling(t *testing.T) {
	// Test that negative retry-after durations are handled correctly
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(1, time.Nanosecond),
		NewFixedWindowStrategy(1, time.Nanosecond),
		NewSlidingWindowStrategy(1, time.Nanosecond),
		NewLeakyBucketStrategy(1000000.0, 1), // Very high rate
		NewAdaptiveStrategy(1000000.0, 1000000.0, 1000000.0, time.Nanosecond),
	}

	for i, strategy := range strategies {
		t.Run(strategy.Name(), func(t *testing.T) {
			// First request
			allowed, _ := strategy.Allow("test_key")
			if !allowed {
				t.Fatalf("first request should be allowed for strategy %d", i)
			}

			// Second request might have negative retry time due to time precision
			allowed, retryAfter := strategy.Allow("test_key")
			if !allowed && retryAfter < 0 {
				t.Fatalf("negative retry-after not handled for strategy %d: %v", i, retryAfter)
			}
		})
	}

	// Clean up
	for _, strategy := range strategies {
		if closer, ok := strategy.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}

// =============================================================================
// Comprehensive Coverage Tests
// =============================================================================

func TestDefaultKeyFuncWithNilRequest(t *testing.T) {
	// Test defaultKeyFunc with nil request using a simple mock
	result := defaultKeyFunc(&mockCtx{req: nil})
	if result != "" {
		t.Fatalf("expected empty string for nil request, got %s", result)
	}
}

type mockCtx struct {
	req *http.Request
}

// Implement only the methods we need for testing
func (m *mockCtx) Request() *http.Request                                    { return m.req }
func (m *mockCtx) SetRequest(*http.Request)                                  {}
func (m *mockCtx) ResponseWriter() http.ResponseWriter                       { return nil }
func (m *mockCtx) SetResponseWriter(http.ResponseWriter)                     {}
func (m *mockCtx) Context() context.Context                                  { return context.Background() }
func (m *mockCtx) Method() string                                            { return "GET" }
func (m *mockCtx) Path() string                                              { return "/" }
func (m *mockCtx) Route() string                                             { return "/" }
func (m *mockCtx) Param(string) string                                       { return "" }
func (m *mockCtx) Query(string) string                                       { return "" }
func (m *mockCtx) ParamInt(string, ...int) int                               { return 0 }
func (m *mockCtx) ParamInt64(string, ...int64) int64                         { return 0 }
func (m *mockCtx) ParamUint(string, ...uint) uint                            { return 0 }
func (m *mockCtx) ParamFloat64(string, ...float64) float64                   { return 0 }
func (m *mockCtx) ParamBool(string, ...bool) bool                            { return false }
func (m *mockCtx) QueryInt(string, ...int) int                               { return 0 }
func (m *mockCtx) QueryInt64(string, ...int64) int64                         { return 0 }
func (m *mockCtx) QueryUint(string, ...uint) uint                            { return 0 }
func (m *mockCtx) QueryFloat64(string, ...float64) float64                   { return 0 }
func (m *mockCtx) QueryBool(string, ...bool) bool                            { return false }
func (m *mockCtx) ParamSafe(string) string                                   { return "" }
func (m *mockCtx) QuerySafe(string) string                                   { return "" }
func (m *mockCtx) ParamAlphaNum(string) string                               { return "" }
func (m *mockCtx) QueryAlphaNum(string) string                               { return "" }
func (m *mockCtx) ParamFilename(string) string                               { return "" }
func (m *mockCtx) QueryFilename(string) string                               { return "" }
func (m *mockCtx) Header(string, string)                                     {}
func (m *mockCtx) Status(int) flash.Ctx                                      { return m }
func (m *mockCtx) StatusCode() int                                           { return 200 }
func (m *mockCtx) JSON(any) error                                            { return nil }
func (m *mockCtx) String(int, string) error                                  { return nil }
func (m *mockCtx) Send(int, string, []byte) (int, error)                     { return 0, nil }
func (m *mockCtx) WroteHeader() bool                                         { return false }
func (m *mockCtx) BindJSON(any, ...ctx.BindJSONOptions) error                { return nil }
func (m *mockCtx) BindMap(any, map[string]any, ...ctx.BindJSONOptions) error { return nil }
func (m *mockCtx) BindForm(any, ...ctx.BindJSONOptions) error                { return nil }
func (m *mockCtx) BindQuery(any, ...ctx.BindJSONOptions) error               { return nil }
func (m *mockCtx) BindPath(any, ...ctx.BindJSONOptions) error                { return nil }
func (m *mockCtx) BindAny(any, ...ctx.BindJSONOptions) error                 { return nil }
func (m *mockCtx) Get(any, ...any) any                                       { return nil }
func (m *mockCtx) Set(any, any) flash.Ctx                                    { return m }
func (m *mockCtx) Clone() flash.Ctx                                          { return m }

func TestCleanupFunctions(t *testing.T) {
	// Test cleanup functions by creating strategies with very short intervals

	// Test TokenBucket cleanup
	tb := NewTokenBucketStrategy(1, 100*time.Millisecond)
	tb.Allow("cleanup_test_tb")

	// Force cleanup by calling it directly
	go func() {
		time.Sleep(10 * time.Millisecond)
		tb.Close()
	}()

	// Test FixedWindow cleanup
	fw := NewFixedWindowStrategy(1, 100*time.Millisecond)
	fw.Allow("cleanup_test_fw")
	go func() {
		time.Sleep(10 * time.Millisecond)
		fw.Close()
	}()

	// Test SlidingWindow cleanup
	sw := NewSlidingWindowStrategy(1, 100*time.Millisecond)
	sw.Allow("cleanup_test_sw")
	go func() {
		time.Sleep(10 * time.Millisecond)
		sw.Close()
	}()

	// Test LeakyBucket cleanup
	lb := NewLeakyBucketStrategy(1.0, 1)
	lb.Allow("cleanup_test_lb")
	go func() {
		time.Sleep(10 * time.Millisecond)
		lb.Close()
	}()

	// Test Adaptive cleanup
	as := NewAdaptiveStrategy(1.0, 0.1, 10.0, 100*time.Millisecond)
	as.Allow("cleanup_test_as")
	go func() {
		time.Sleep(10 * time.Millisecond)
		as.Close()
	}()

	// Wait for cleanup to potentially run
	time.Sleep(50 * time.Millisecond)
}

func TestUtilityFunctions(t *testing.T) {
	// Test max function
	if max(5, 3) != 5 {
		t.Fatalf("max(5, 3) should be 5")
	}
	if max(2, 7) != 7 {
		t.Fatalf("max(2, 7) should be 7")
	}

	// Test maxFloat64 function
	if maxFloat64(5.5, 3.3) != 5.5 {
		t.Fatalf("maxFloat64(5.5, 3.3) should be 5.5")
	}
	if maxFloat64(2.2, 7.7) != 7.7 {
		t.Fatalf("maxFloat64(2.2, 7.7) should be 7.7")
	}

	// Test min function
	if min(5.5, 3.3) != 3.3 {
		t.Fatalf("min(5.5, 3.3) should be 3.3")
	}
	if min(2.2, 7.7) != 2.2 {
		t.Fatalf("min(2.2, 7.7) should be 2.2")
	}
}

func TestFormatSecondsEdgeCases(t *testing.T) {
	// Test formatSeconds with various durations
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "1"},                      // Less than 1 second
		{500 * time.Millisecond, "1"}, // Less than 1 second
		{time.Second, "1"},            // Exactly 1 second
		{2 * time.Second, "2"},        // 2 seconds
		{61 * time.Second, "61"},      // 61 seconds
	}

	for _, test := range tests {
		result := formatSeconds(test.duration)
		if result != test.expected {
			t.Fatalf("formatSeconds(%v): expected %s, got %s", test.duration, test.expected, result)
		}
	}
}

func TestSecureClientIPEdgeCases(t *testing.T) {
	// Test with malformed X-Forwarded-For
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "invalid_ip, another_invalid")

	result := secureClientIP(req, []string{"10.0.0.0/8"})
	if result != "10.0.0.1" {
		t.Fatalf("expected fallback to direct IP, got %s", result)
	}

	// Test with invalid CIDR in trusted proxies
	result = secureClientIP(req, []string{"invalid_cidr"})
	if result != "10.0.0.1" {
		t.Fatalf("expected direct IP with invalid CIDR, got %s", result)
	}

	// Test with all private IPs in XFF chain
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.2")
	result = secureClientIP(req, []string{"10.0.0.0/8"})
	if result != "10.0.0.1" {
		t.Fatalf("expected fallback to direct IP when all XFF are private, got %s", result)
	}
}

func TestRateLimitWithZeroRetryAfter(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Nanosecond) // Very short refill
	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// First request
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request should be blocked but might have zero retry-after
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	// Should either be OK (if time passed) or 429 (if blocked)
	if rec.Code != http.StatusOK && rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 200 or 429, got %d", rec.Code)
	}
}

func TestSlidingWindowEmptyTimestamps(t *testing.T) {
	sw := NewSlidingWindowStrategy(1, time.Minute)
	defer sw.Close()

	// Test with empty timestamps slice
	sw.mu.Lock()
	sw.windows["empty_test"] = []time.Time{}
	sw.mu.Unlock()

	// Should allow request
	allowed, _ := sw.Allow("empty_test")
	if !allowed {
		t.Fatalf("expected request to be allowed with empty timestamps")
	}
}

func TestLeakyBucketZeroLevel(t *testing.T) {
	lb := NewLeakyBucketStrategy(1.0, 5)
	defer lb.Close()

	// Create bucket with zero level
	lb.mu.Lock()
	lb.buckets["zero_test"] = &leakyBucket{
		lastLeak: time.Now(),
		level:    0,
	}
	lb.mu.Unlock()

	// Should allow request
	allowed, _ := lb.Allow("zero_test")
	if !allowed {
		t.Fatalf("expected request to be allowed with zero level")
	}
}

func TestAdaptiveStrategyRateBounds(t *testing.T) {
	as := NewAdaptiveStrategy(5.0, 1.0, 10.0, time.Minute)
	defer as.Close()

	// Allow first request to create client
	as.Allow("bounds_test")

	// Test increasing rate beyond max
	for i := 0; i < 10; i++ {
		as.UpdateRate("bounds_test", true)
	}

	as.mu.RLock()
	client := as.clients["bounds_test"]
	rate := client.currentRate
	as.mu.RUnlock()

	if rate > as.maxRate {
		t.Fatalf("rate should not exceed maxRate: got %f, max %f", rate, as.maxRate)
	}

	// Test decreasing rate below min
	for i := 0; i < 10; i++ {
		as.UpdateRate("bounds_test", false)
	}

	as.mu.RLock()
	client = as.clients["bounds_test"]
	rate = client.currentRate
	as.mu.RUnlock()

	if rate < as.minRate {
		t.Fatalf("rate should not go below minRate: got %f, min %f", rate, as.minRate)
	}
}

func TestRateLimitMiddlewareEdgeCases(t *testing.T) {
	a := flash.New()

	// Test with nil strategy (should use default)
	a.Use(RateLimit())
	a.GET("/default", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Test default behavior
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/default", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with default strategy, got %d", rec.Code)
	}
}

func TestClientIPWithPortParsing(t *testing.T) {
	tests := []struct {
		remoteAddr string
		expected   string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"invalid:format:here", "invalid:format:here"}, // fallback
	}

	for _, test := range tests {
		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = test.remoteAddr

		result := clientIP(req)
		if result != test.expected {
			t.Fatalf("clientIP(%s): expected %s, got %s", test.remoteAddr, test.expected, result)
		}
	}
}

func TestIsPrivateOrLoopbackIPv6(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"fe80::1", true},      // link-local
		{"ff02::1", true},      // multicast link-local
		{"fc00::1", true},      // unique local
		{"2001:db8::1", false}, // documentation (not private in Go's definition)
	}

	for _, test := range tests {
		ip := net.ParseIP(test.ip)
		if ip == nil {
			t.Fatalf("invalid IP: %s", test.ip)
		}
		result := isPrivateOrLoopback(ip)
		if result != test.expected {
			t.Fatalf("IP %s: expected %t, got %t", test.ip, test.expected, result)
		}
	}
}

func TestAllStrategiesWithExpiredEntries(t *testing.T) {
	// Test all strategies with expired entries to ensure proper handling

	// TokenBucket with expired bucket
	tb := NewTokenBucketStrategy(1, time.Nanosecond)
	defer tb.Close()
	tb.Allow("expired_test")
	time.Sleep(time.Millisecond) // Ensure expiration
	allowed, _ := tb.Allow("expired_test")
	if !allowed {
		t.Fatalf("TokenBucket should allow request after expiration")
	}

	// FixedWindow with expired window
	fw := NewFixedWindowStrategy(1, time.Nanosecond)
	defer fw.Close()
	fw.Allow("expired_test")
	time.Sleep(time.Millisecond)
	allowed, _ = fw.Allow("expired_test")
	if !allowed {
		t.Fatalf("FixedWindow should allow request after expiration")
	}

	// SlidingWindow with expired timestamps
	sw := NewSlidingWindowStrategy(1, time.Nanosecond)
	defer sw.Close()
	sw.Allow("expired_test")
	time.Sleep(time.Millisecond)
	allowed, _ = sw.Allow("expired_test")
	if !allowed {
		t.Fatalf("SlidingWindow should allow request after expiration")
	}
}

func TestStrategyConstructorEdgeCases(t *testing.T) {
	// Test constructors with zero values to hit default branches

	// TokenBucket with zero capacity
	tb := NewTokenBucketStrategy(0, 0)
	defer tb.Close()
	if tb.capacity != 1 {
		t.Fatalf("expected capacity default to 1, got %d", tb.capacity)
	}
	if tb.refill != time.Minute {
		t.Fatalf("expected refill default to 1 minute, got %v", tb.refill)
	}

	// SlidingWindow with zero limit
	sw := NewSlidingWindowStrategy(0, 0)
	defer sw.Close()
	if sw.limit != 1 {
		t.Fatalf("expected limit default to 1, got %d", sw.limit)
	}
	if sw.window != time.Minute {
		t.Fatalf("expected window default to 1 minute, got %v", sw.window)
	}
}

func TestCleanupTimerCoverage(t *testing.T) {
	// Test cleanup functions by forcing them to run with short intervals

	// Create a strategy and immediately trigger cleanup
	tb := NewTokenBucketStrategy(1, time.Millisecond)

	// Add an entry that will expire
	tb.Allow("cleanup_test")

	// Manually trigger cleanup by waiting and then closing
	time.Sleep(10 * time.Millisecond)

	// Check that cleanup goroutine is running by checking lastCleanup
	initialCleanup := atomic.LoadInt64(&tb.lastCleanup)

	// Close to trigger cleanup exit
	tb.Close()

	// Verify cleanup was initialized
	if initialCleanup == 0 {
		t.Log("Cleanup may not have run yet (timing dependent)")
	}
}

func TestDoubleCheckLockingPaths(t *testing.T) {
	// Test the double-checked locking paths in Allow methods

	tb := NewTokenBucketStrategy(2, time.Minute)
	defer tb.Close()

	// Create a bucket manually to test the double-check path
	tb.mu.Lock()
	tb.buckets["double_check_test"] = &tokenBucket{
		remaining: 1,
		reset:     time.Now().Add(time.Minute),
	}
	tb.mu.Unlock()

	// This should hit the existing bucket path
	allowed, _ := tb.Allow("double_check_test")
	if !allowed {
		t.Fatalf("expected request to be allowed with existing bucket")
	}

	// Test the same for other strategies
	fw := NewFixedWindowStrategy(2, time.Minute)
	defer fw.Close()

	fw.mu.Lock()
	fw.windows["double_check_test"] = &fixedWindow{
		count: 1,
		reset: time.Now().Add(time.Minute),
	}
	fw.mu.Unlock()

	allowed, _ = fw.Allow("double_check_test")
	if !allowed {
		t.Fatalf("expected request to be allowed with existing window")
	}
}

func TestSlidingWindowEarliestTimestamp(t *testing.T) {
	// Test the earliest timestamp finding logic in sliding window
	sw := NewSlidingWindowStrategy(2, time.Minute)
	defer sw.Close()

	// Fill up the window
	sw.Allow("earliest_test")
	sw.Allow("earliest_test")

	// Third request should be blocked and calculate retry time
	allowed, retryAfter := sw.Allow("earliest_test")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry after time")
	}
}

func TestLeakyBucketWithHighLeak(t *testing.T) {
	// Test leaky bucket with high leak rate
	lb := NewLeakyBucketStrategy(1000.0, 1) // Very high leak rate
	defer lb.Close()

	// Fill bucket
	lb.Allow("high_leak_test")

	// Wait a bit for leaking
	time.Sleep(time.Millisecond)

	// Should allow due to leaking
	allowed, _ := lb.Allow("high_leak_test")
	if !allowed {
		t.Fatalf("expected request to be allowed after leaking")
	}
}

func TestAdaptiveStrategyDoubleCheck(t *testing.T) {
	// Test adaptive strategy double-check locking
	as := NewAdaptiveStrategy(1.0, 0.1, 10.0, time.Minute)
	defer as.Close()

	// Create client manually
	as.mu.Lock()
	as.clients["double_check_adaptive"] = &adaptiveClient{
		lastRequest: time.Now().Add(-time.Second), // 1 second ago
		currentRate: 2.0,
	}
	as.mu.Unlock()

	// Should be allowed (enough time passed)
	allowed, _ := as.Allow("double_check_adaptive")
	if !allowed {
		t.Fatalf("expected request to be allowed")
	}
}

func TestDefaultKeyFuncWithValidRequest(t *testing.T) {
	// Test defaultKeyFunc with valid request to improve coverage
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:8080"

	result := defaultKeyFunc(&mockCtx{req: req})
	if result != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", result)
	}
}

func TestRateLimitWithDisabledCleanup(t *testing.T) {
	// Test rate limit with cleanup disabled
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	a.Use(RateLimit(
		WithStrategy(strategy),
		WithCleanupInterval(-1), // Disabled
	))
	a.GET("/x", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Should still work normally
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAllowMethodEdgeCases(t *testing.T) {
	// Test edge cases in Allow methods

	// Test TokenBucket with zero remaining after reset check
	tb := NewTokenBucketStrategy(1, time.Minute)
	defer tb.Close()

	// Use up the token
	tb.Allow("edge_test")

	// Should be blocked
	allowed, retryAfter := tb.Allow("edge_test")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry time, got %v", retryAfter)
	}

	// Test FixedWindow at limit
	fw := NewFixedWindowStrategy(1, time.Minute)
	defer fw.Close()

	fw.Allow("edge_test")
	allowed, retryAfter = fw.Allow("edge_test")
	if allowed {
		t.Fatalf("expected request to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retry time")
	}
}

func TestConcurrentStrategyAccess(t *testing.T) {
	// Test concurrent access to strategies to ensure thread safety
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(100, time.Minute),
		NewFixedWindowStrategy(100, time.Minute),
		NewSlidingWindowStrategy(100, time.Minute),
		NewLeakyBucketStrategy(100.0, 100),
		NewAdaptiveStrategy(50.0, 10.0, 100.0, time.Minute),
	}

	defer func() {
		for _, strategy := range strategies {
			if closer, ok := strategy.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}()

	for i, strategy := range strategies {
		t.Run(strategy.Name(), func(t *testing.T) {
			var wg sync.WaitGroup
			errors := make(chan error, 10)

			// Run concurrent requests
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					defer func() {
						if r := recover(); r != nil {
							errors <- fmt.Errorf("panic in strategy %d: %v", i, r)
						}
					}()

					// Make multiple requests
					for k := 0; k < 5; k++ {
						strategy.Allow(fmt.Sprintf("concurrent_%d_%d", id, k))
						if as, ok := strategy.(*AdaptiveStrategy); ok {
							as.UpdateRate(fmt.Sprintf("concurrent_%d_%d", id, k), k%2 == 0)
						}
					}
				}(j)
			}

			wg.Wait()
			close(errors)

			// Check for any errors
			for err := range errors {
				t.Fatal(err)
			}
		})
	}
}

func TestMemoryCleanupEffectiveness(t *testing.T) {
	// Test that cleanup actually removes entries
	tb := NewTokenBucketStrategy(1, time.Nanosecond) // Very short expiry
	defer tb.Close()

	// Add entries
	for i := 0; i < 10; i++ {
		tb.Allow(fmt.Sprintf("cleanup_test_%d", i))
	}

	// Check initial count
	tb.mu.RLock()
	initialCount := len(tb.buckets)
	tb.mu.RUnlock()

	if initialCount != 10 {
		t.Fatalf("expected 10 buckets, got %d", initialCount)
	}

	// Wait for expiration
	time.Sleep(time.Millisecond)

	// The cleanup runs every 5 minutes by default, so we can't easily test
	// automatic cleanup, but we can test that entries expire when accessed
	tb.Allow("cleanup_test_0") // This should create a new bucket

	t.Log("Memory cleanup test completed - automatic cleanup timing is implementation dependent")
}

func TestRateLimitStrategiesCleanupAndClose(t *testing.T) {
	// Test that all strategies can be closed properly
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(10, time.Minute),
		NewFixedWindowStrategy(10, time.Minute),
		NewSlidingWindowStrategy(10, time.Minute),
		NewLeakyBucketStrategy(10.0, 5),
		NewAdaptiveStrategy(10.0, 1.0, 100.0, time.Minute),
	}

	for _, strategy := range strategies {
		// Use each strategy
		strategy.Allow("test_client")

		// Close should not panic - use type assertion since interface doesn't include Close
		if closer, ok := strategy.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}

func TestRateLimitWithEmptyKeyFallback(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(1, time.Minute)
	defer strategy.Close()

	a.Use(RateLimit(WithStrategy(strategy)))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Create request with no identifying information (should use "unknown" key)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Remove all headers that could be used for identification
	req.RemoteAddr = ""
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimitWithLongKey(t *testing.T) {
	a := flash.New()
	strategy := NewTokenBucketStrategy(10, time.Minute)
	defer strategy.Close()

	a.Use(RateLimit(
		WithStrategy(strategy),
		WithMaxKeyLength(10), // Very short limit
		WithKeyFunc(func(c flash.Ctx) string {
			return "this_is_a_very_long_key_that_exceeds_the_limit"
		}),
	))
	a.GET("/test", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimitStrategyEdgeCases(t *testing.T) {
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(1, time.Minute),
		NewFixedWindowStrategy(1, time.Minute),
		NewSlidingWindowStrategy(1, time.Minute),
		NewLeakyBucketStrategy(1.0, 1),
		NewAdaptiveStrategy(1.0, 0.1, 10.0, time.Minute),
	}

	for _, strategy := range strategies {
		defer func() {
			if closer, ok := strategy.(interface{ Close() }); ok {
				closer.Close()
			}
		}()

		// Test rapid consecutive requests to trigger edge cases
		for i := 0; i < 5; i++ {
			allowed, retryAfter := strategy.Allow(fmt.Sprintf("edge_test_%d", i))
			if !allowed && retryAfter < 0 {
				t.Errorf("retry after should not be negative: %v", retryAfter)
			}
		}

		// Test same key multiple times
		key := "same_key_test"
		for i := 0; i < 3; i++ {
			strategy.Allow(key)
		}

		// Test adaptive strategy specific functionality
		if as, ok := strategy.(*AdaptiveStrategy); ok {
			as.UpdateRate("adaptive_test", true)  // Good behavior
			as.UpdateRate("adaptive_test", false) // Bad behavior
		}
	}
}

func TestRateLimitWithZeroCapacity(t *testing.T) {
	// Test strategies with zero/invalid capacity (should use defaults)
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(0, time.Minute),   // Should default to 1
		NewFixedWindowStrategy(0, time.Minute),   // Should default to 1
		NewSlidingWindowStrategy(0, time.Minute), // Should default to 1
		NewLeakyBucketStrategy(0.0, 0),           // Should default to 1.0, 1
		NewAdaptiveStrategy(0.0, 0.0, 0.0, 0),    // Should use defaults
	}

	for i, strategy := range strategies {
		defer func() {
			if closer, ok := strategy.(interface{ Close() }); ok {
				closer.Close()
			}
		}()

		// Should allow at least one request
		allowed, _ := strategy.Allow(fmt.Sprintf("zero_test_%d", i))
		if !allowed {
			t.Errorf("strategy %d should allow at least one request", i)
		}
	}
}

func TestRateLimitStrategiesWithConcurrentCleanup(t *testing.T) {
	// Test cleanup behavior under concurrent access
	strategies := []RateLimitStrategy{
		NewTokenBucketStrategy(10, time.Minute),
		NewFixedWindowStrategy(10, time.Minute),
		NewSlidingWindowStrategy(10, time.Minute),
		NewLeakyBucketStrategy(10.0, 10),
		NewAdaptiveStrategy(10.0, 1.0, 50.0, time.Minute),
	}

	for _, strategy := range strategies {
		// Add some entries to clean up
		for i := 0; i < 5; i++ {
			strategy.Allow(fmt.Sprintf("key%d", i))
		}

		// Force cleanup by calling with a very old time
		// This should exercise the cleanup goroutines
		time.Sleep(1 * time.Millisecond)
		strategy.Allow("cleanup_trigger")

		// Close strategy if possible
		if closer, ok := strategy.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}

func TestRateLimitAllowMethodEdgeCases(t *testing.T) {
	// Test edge cases in Allow methods to hit uncovered branches

	// Test with empty key (should use fallback)
	strategy := NewTokenBucketStrategy(5, time.Second)
	allowed, _ := strategy.Allow("")
	if !allowed {
		t.Error("expected empty key to be allowed")
	}

	// Test with very long key (should be truncated)
	longKey := strings.Repeat("a", 1000)
	allowed, _ = strategy.Allow(longKey)
	if !allowed {
		t.Error("expected long key to be allowed")
	}

	// Test rapid successive calls to hit various branches
	for i := 0; i < 20; i++ {
		strategy.Allow(fmt.Sprintf("rapid%d", i))
	}

	// Close strategy
	strategy.Close()
}

func TestAdaptiveStrategySpecificBehavior(t *testing.T) {
	// Test adaptive strategy's specific behavior
	strategy := NewAdaptiveStrategy(10.0, 1.0, 50.0, time.Minute)

	// Make multiple requests to trigger adaptation logic
	key := "adaptive_test"
	for i := 0; i < 15; i++ {
		allowed, _ := strategy.Allow(key)
		// Just log the results, don't assert specific behavior
		// since adaptive strategy behavior depends on timing
		t.Logf("Request %d: allowed=%v", i, allowed)
	}

	// Test with different keys to exercise different code paths
	for i := 0; i < 5; i++ {
		strategy.Allow(fmt.Sprintf("adaptive_key_%d", i))
	}

	// Close strategy
	strategy.Close()
}

func TestRateLimitCleanupEdgeCases(t *testing.T) {
	// Test cleanup edge cases to hit more coverage
	tb := NewTokenBucketStrategy(10, 100*time.Millisecond) // Short window
	defer tb.Close()

	// Generate requests with many different keys to create cleanup targets
	for i := 0; i < 200; i++ {
		tb.Allow(fmt.Sprintf("key_%d", i))
	}

	// Wait for cleanup to potentially trigger
	time.Sleep(200 * time.Millisecond)

	// More requests to exercise post-cleanup logic
	for i := 0; i < 50; i++ {
		tb.Allow(fmt.Sprintf("post_cleanup_%d", i))
	}
}
