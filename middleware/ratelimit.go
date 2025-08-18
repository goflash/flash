package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goflash/flash/v1"
)

// Limiter is a generic interface for rate limiting.
// Allow returns whether the request is allowed and, if not, how long to wait before retrying.
type Limiter interface {
	Allow(key string) (allowed bool, retryAfter time.Duration)
}

// SimpleIPLimiter is a basic token bucket per-IP limiter.
// It limits requests per unique key (e.g., IP address) with a fixed capacity and refill interval.
type SimpleIPLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity int
	fill     time.Duration
}

type bucket struct {
	remaining int
	reset     time.Time
}

// NewSimpleIPLimiter creates a new SimpleIPLimiter with the given capacity and refill interval.
func NewSimpleIPLimiter(capacity int, refill time.Duration) *SimpleIPLimiter {
	return &SimpleIPLimiter{buckets: map[string]*bucket{}, capacity: capacity, fill: refill}
}

func (l *SimpleIPLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	b := l.buckets[key]
	if b == nil || time.Now().After(b.reset) {
		b = &bucket{remaining: l.capacity - 1, reset: time.Now().Add(l.fill)}
		l.buckets[key] = b
		l.mu.Unlock()
		return true, 0
	}
	if b.remaining > 0 {
		b.remaining--
		l.mu.Unlock()
		return true, 0
	}
	retry := time.Until(b.reset)
	l.mu.Unlock()
	return false, retry
}

// RateLimit returns middleware that uses the provided Limiter to restrict requests.
// If the request is not allowed, responds with 429 Too Many Requests and optional Retry-After header.
func RateLimit(l Limiter) flash.Middleware {
	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			ip := ""
			if r := c.Request(); r != nil {
				ip = clientIP(r)
			}
			if ok, retry := l.Allow(ip); !ok {
				if retry > 0 {
					c.Header("Retry-After", formatSeconds(retry))
				}
				return c.String(http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
			}
			return next(c)
		}
	}
}

// clientIP extracts a stable client identifier for rate limiting.
// Preference order: X-Forwarded-For (first), X-Real-IP, RemoteAddr host part.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may contain multiple IPs: client, proxy1, proxy2
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	// Fallback to RemoteAddr as-is if it cannot be split
	return r.RemoteAddr
}

func formatSeconds(d time.Duration) string {
	sec := int(d.Seconds())
	if sec < 1 {
		sec = 1
	}
	return strconv.Itoa(sec)
}
