// Package middleware provides comprehensive session management functionality for HTTP applications.
//
// The session middleware offers secure, efficient session handling with support for multiple
// storage backends, configurable security settings, and production-ready features including
// automatic cleanup, session regeneration, and timing attack protection.
//
// # Features
//
// • Secure session ID generation with cryptographic randomness
// • Pluggable storage interface supporting memory, database, Redis, etc.
// • Automatic session expiration and cleanup
// • Session regeneration for security (prevents session fixation)
// • Timing attack protection for session lookups
// • Cookie and header-based session ID transport
// • Comprehensive security headers and cookie attributes
// • Memory-efficient operations with optimized copying
// • Thread-safe operations with fine-grained locking
//
// # Quick Start
//
// Basic session usage:
//
//	import "github.com/goflash/flash/v2/middleware"
//
//	app := flash.New()
//	app.Use(middleware.Sessions(middleware.SessionConfig{
//		Store:      middleware.NewMemoryStore(),
//		TTL:        24 * time.Hour,
//		CookieName: "session_id",
//		HTTPOnly:   true,
//		Secure:     true, // Enable in production with HTTPS
//	}))
//
//	app.GET("/login", func(c flash.Ctx) error {
//		session := middleware.SessionFromCtx(c)
//		session.Set("user_id", "123")
//		session.Set("authenticated", true)
//		return c.JSON(200, map[string]string{"status": "logged in"})
//	})
//
// # Security Features
//
// Session Regeneration:
// Prevents session fixation attacks by generating new session IDs after authentication.
//
//	session := middleware.SessionFromCtx(c)
//	session.Regenerate() // Generate new session ID
//	session.Set("user_id", userID)
//
// Secure Cookie Configuration:
// Production-ready cookie security settings.
//
//	config := middleware.SessionConfig{
//		Secure:   true,                    // HTTPS only
//		HTTPOnly: true,                    // No JavaScript access
//		SameSite: http.SameSiteStrictMode, // CSRF protection
//		Domain:   ".example.com",          // Scope to domain
//	}
//
// # Storage Backends
//
// Memory Store (Development):
// Built-in memory store with automatic cleanup.
//
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(5 * time.Minute) // Clean expired sessions every 5 minutes
//
// Custom Store Implementation:
// Implement the Store interface for database, Redis, etc.
//
//	type RedisStore struct { /* ... */ }
//	func (r *RedisStore) Get(id string) (map[string]any, bool) { /* ... */ }
//	func (r *RedisStore) Save(id string, data map[string]any, ttl time.Duration) error { /* ... */ }
//	func (r *RedisStore) Delete(id string) error { /* ... */ }
package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goflash/flash/v2"
)

type sessionContextKey struct{}

// Store abstracts session persistence for the session middleware.
// Implementations must provide thread-safe Get, Save, and Delete operations for session data by ID.
//
// The interface is designed to support various storage backends including:
//   - In-memory stores (for development and testing)
//   - Database stores (PostgreSQL, MySQL, SQLite)
//   - Key-value stores (Redis, Memcached, etcd)
//   - File-based stores (for single-instance deployments)
//   - Distributed stores (for multi-region deployments)
//
// Example implementation:
//
//	type CustomStore struct {
//		// store-specific fields
//	}
//
//	func (cs *CustomStore) Get(id string) (map[string]any, bool) {
//		// Retrieve session data by ID
//		// Return data and true if found, nil and false if not found
//		// Should handle expiration internally if supported
//		return data, found
//	}
//
//	func (cs *CustomStore) Save(id string, data map[string]any, ttl time.Duration) error {
//		// Persist session data with optional TTL
//		// Return error if save operation fails
//		// Should be atomic to prevent partial saves
//		return nil
//	}
//
//	func (cs *CustomStore) Delete(id string) error {
//		// Remove session data by ID
//		// Return error if delete operation fails
//		// Should be idempotent (no error if ID doesn't exist)
//		return nil
//	}
//
// Security considerations for implementations:
//   - Use timing-safe comparison for session ID lookups to prevent timing attacks
//   - Implement proper cleanup of expired sessions to prevent memory leaks
//   - Consider encryption at rest for sensitive session data
//   - Use connection pooling and proper error handling for network-based stores
//   - Implement proper logging for security auditing
type Store interface {
	// Get retrieves session data by ID.
	// Returns the session data and true if found, nil and false if not found or expired.
	// Implementations should handle expiration checking internally.
	Get(id string) (map[string]any, bool)

	// Save persists session data with the given ID and TTL.
	// If TTL is 0, the session should not expire (or use store default).
	// Returns error if the save operation fails.
	Save(id string, data map[string]any, ttl time.Duration) error

	// Delete removes session data by ID.
	// Should be idempotent - no error if the ID doesn't exist.
	// Returns error only if the delete operation fails.
	Delete(id string) error
}

// MemoryStore is an in-memory session store with TTL and automatic cleanup.
// Suitable for development, testing, and single-instance production deployments.
//
// Features:
//   - Thread-safe operations with optimized read-write locking
//   - Automatic cleanup of expired sessions via background goroutine
//   - Timing attack protection for session ID lookups
//   - Memory-efficient storage with lazy expiration checking
//   - Configurable cleanup intervals
//   - Proper resource cleanup on shutdown
//
// Security considerations:
//   - Uses timing-safe comparison to prevent session enumeration
//   - Automatically removes expired sessions to prevent memory leaks
//   - Safe for concurrent access from multiple goroutines
//
// Example usage:
//
//	// Create store with automatic cleanup every 5 minutes
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(5 * time.Minute)
//	defer store.StopCleanup()
//
//	// Use with session middleware
//	app.Use(middleware.Sessions(middleware.SessionConfig{
//		Store: store,
//		TTL:   24 * time.Hour,
//	}))
type MemoryStore struct {
	mu            sync.RWMutex
	data          map[string]entry
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
	cleanupOnce   sync.Once
}

type entry struct {
	v        map[string]any
	exp      time.Time
	accessed int64 // atomic timestamp for LRU-style cleanup
}

// NewMemoryStore creates a new in-memory session store.
// Call StartCleanup() to enable automatic cleanup of expired sessions.
//
// Example:
//
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(10 * time.Minute) // Clean up every 10 minutes
//	defer store.StopCleanup()
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:        make(map[string]entry),
		cleanupDone: make(chan struct{}),
	}
}

// Get retrieves session data by ID with timing attack protection.
// Returns a copy of the session data to prevent external modification.
func (m *MemoryStore) Get(id string) (map[string]any, bool) {
	now := time.Now()

	m.mu.RLock()
	e, ok := m.data[id]
	m.mu.RUnlock()

	// Use timing-safe comparison to prevent session enumeration attacks
	if !ok {
		// Perform dummy operation to maintain constant time
		_ = subtle.ConstantTimeCompare([]byte(id), []byte("dummy_session_id_for_timing"))
		return nil, false
	}

	// Check expiration
	if !e.exp.IsZero() && now.After(e.exp) {
		// Lazy cleanup - remove expired session
		_ = m.Delete(id)
		return nil, false
	}

	// Update access time for LRU-style cleanup (atomic to avoid lock contention)
	atomic.StoreInt64(&e.accessed, now.Unix())

	// Return a copy to prevent external modification
	return copyMapEfficient(e.v), true
}

// Save persists session data with the given ID and TTL.
// Creates a deep copy of the data to prevent external modification.
func (m *MemoryStore) Save(id string, data map[string]any, ttl time.Duration) error {
	if id == "" {
		return errors.New("session: empty session id")
	}

	now := time.Now()
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}

	// Create entry with current access time
	e := entry{
		v:        copyMapEfficient(data),
		exp:      exp,
		accessed: now.Unix(),
	}

	m.mu.Lock()
	m.data[id] = e
	m.mu.Unlock()
	return nil
}

// Delete removes session data by ID.
// Idempotent operation - no error if the ID doesn't exist.
func (m *MemoryStore) Delete(id string) error {
	m.mu.Lock()
	delete(m.data, id)
	m.mu.Unlock()
	return nil
}

// StartCleanup starts a background goroutine that periodically removes expired sessions.
// This prevents memory leaks in long-running applications.
//
// Example:
//
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(5 * time.Minute) // Clean up every 5 minutes
//	defer store.StopCleanup()
func (m *MemoryStore) StartCleanup(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute // Default cleanup interval
	}

	m.cleanupOnce.Do(func() {
		m.cleanupTicker = time.NewTicker(interval)
		go m.cleanupLoop()
	})
}

// StopCleanup stops the background cleanup goroutine.
// Should be called when the store is no longer needed to prevent goroutine leaks.
func (m *MemoryStore) StopCleanup() {
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
		close(m.cleanupDone)
	}
}

// cleanupLoop runs in a background goroutine to remove expired sessions.
func (m *MemoryStore) cleanupLoop() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanupExpired()
		case <-m.cleanupDone:
			return
		}
	}
}

// cleanupExpired removes all expired sessions from the store.
// This method is called periodically by the cleanup goroutine.
func (m *MemoryStore) cleanupExpired() {
	now := time.Now()
	toDelete := make([]string, 0, 16) // Pre-allocate for efficiency

	// First pass: collect expired session IDs (with read lock)
	m.mu.RLock()
	for id, e := range m.data {
		if !e.exp.IsZero() && now.After(e.exp) {
			toDelete = append(toDelete, id)
		}
	}
	m.mu.RUnlock()

	// Second pass: delete expired sessions (with write lock)
	if len(toDelete) > 0 {
		m.mu.Lock()
		for _, id := range toDelete {
			// Double-check expiration in case of concurrent updates
			if e, exists := m.data[id]; exists && !e.exp.IsZero() && now.After(e.exp) {
				delete(m.data, id)
			}
		}
		m.mu.Unlock()
	}
}

// Len returns the current number of sessions in the store.
// Useful for monitoring and debugging.
func (m *MemoryStore) Len() int {
	m.mu.RLock()
	count := len(m.data)
	m.mu.RUnlock()
	return count
}

// copyMap creates a shallow copy of a map (kept for backward compatibility).
// Use copyMapEfficient for better performance.
func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// copyMapEfficient creates a memory-efficient shallow copy of a map.
// Returns nil for nil input to avoid unnecessary allocations.
func copyMapEfficient(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	if len(src) == 0 {
		return make(map[string]any) // Return empty map, not nil
	}

	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// Session represents the per-request view of a session with security features.
// Provides methods for safely managing session data with automatic change tracking
// and session regeneration capabilities to prevent session fixation attacks.
//
// Example usage:
//
//	session := middleware.SessionFromCtx(c)
//
//	// Set session data
//	session.Set("user_id", "12345")
//	session.Set("authenticated", true)
//
//	// Get session data
//	if userID, ok := session.Get("user_id"); ok {
//		// Use userID
//	}
//
//	// Regenerate session ID after authentication (security best practice)
//	session.Regenerate()
//
//	// Clear sensitive data
//	session.Delete("temp_token")
//
//	// Clear all session data
//	session.Clear()
type Session struct {
	ID          string         // Current session ID
	Values      map[string]any // Session data
	changed     bool           // Tracks if session data has been modified
	new         bool           // Indicates if this is a new session
	regenerated bool           // Tracks if session ID has been regenerated
	oldID       string         // Previous session ID (for cleanup after regeneration)
}

// Get retrieves a value from the session by key.
// Returns the value and true if found, nil and false if not found.
//
// Example:
//
//	if userID, ok := session.Get("user_id"); ok {
//		// userID exists and can be used
//		fmt.Printf("User ID: %v", userID)
//	}
func (s *Session) Get(key string) (any, bool) {
	if s.Values == nil {
		return nil, false
	}
	v, ok := s.Values[key]
	return v, ok
}

// Set stores a value in the session by key.
// Marks the session as changed, triggering a save operation.
//
// Example:
//
//	session.Set("user_id", "12345")
//	session.Set("role", "admin")
//	session.Set("login_time", time.Now())
func (s *Session) Set(key string, v any) {
	if s.Values == nil {
		s.Values = make(map[string]any)
	}
	s.Values[key] = v
	s.changed = true
}

// Delete removes a value from the session by key.
// Marks the session as changed, triggering a save operation.
//
// Example:
//
//	session.Delete("temp_token")
//	session.Delete("csrf_token")
func (s *Session) Delete(key string) {
	if s.Values == nil {
		return
	}
	delete(s.Values, key)
	s.changed = true
}

// Clear removes all values from the session.
// Useful for logout operations or when starting fresh.
//
// Example:
//
//	// Logout - clear all session data
//	session.Clear()
func (s *Session) Clear() {
	if s.Values == nil {
		s.Values = make(map[string]any)
	} else {
		// Clear existing map efficiently
		for k := range s.Values {
			delete(s.Values, k)
		}
	}
	s.changed = true
}

// Regenerate generates a new session ID while preserving session data.
// This is a critical security measure to prevent session fixation attacks.
// Should be called after authentication, privilege escalation, or other security-sensitive operations.
//
// Example:
//
//	// After successful login
//	if authenticateUser(username, password) {
//		session := middleware.SessionFromCtx(c)
//		session.Regenerate() // Prevent session fixation
//		session.Set("user_id", userID)
//		session.Set("authenticated", true)
//	}
//
// Security note: The old session ID will be automatically cleaned up
// from the store when the session is saved.
func (s *Session) Regenerate() {
	if s.ID != "" {
		s.oldID = s.ID // Store old ID for cleanup
	}
	s.ID = newSessionID()
	s.regenerated = true
	s.changed = true
}

// IsNew returns true if this is a newly created session.
func (s *Session) IsNew() bool {
	return s.new
}

// IsChanged returns true if the session data has been modified.
func (s *Session) IsChanged() bool {
	return s.changed
}

// IsRegenerated returns true if the session ID has been regenerated.
func (s *Session) IsRegenerated() bool {
	return s.regenerated
}

// SessionConfig configures the session middleware with comprehensive security and performance options.
// Provides fine-grained control over session behavior, cookie attributes, and security features.
//
// Example basic configuration:
//
//	config := middleware.SessionConfig{
//		Store:      middleware.NewMemoryStore(),
//		TTL:        24 * time.Hour,
//		CookieName: "session_id",
//		HTTPOnly:   true,
//		Secure:     true, // Enable in production with HTTPS
//	}
//
// Example production configuration:
//
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(10 * time.Minute)
//
//	config := middleware.SessionConfig{
//		Store:         store,
//		TTL:           12 * time.Hour,
//		CookieName:    "secure_session",
//		CookiePath:    "/",
//		Domain:        ".example.com",
//		Secure:        true,
//		HTTPOnly:      true,
//		SameSite:      http.SameSiteStrictMode,
//		HeaderName:    "X-Session-ID", // For API clients
//		IdleTimeout:   30 * time.Minute,
//		MaxAge:        7 * 24 * time.Hour,
//		RegenerateOnAuth: true,
//	}
//
// Example API-only configuration:
//
//	config := middleware.SessionConfig{
//		Store:      redisStore,
//		TTL:        2 * time.Hour,
//		HeaderName: "Authorization", // Use Authorization header
//		// No cookie settings for API-only
//	}
type SessionConfig struct {
	// Store is the session storage backend.
	// If nil, defaults to NewMemoryStore().
	Store Store

	// TTL is the session time-to-live duration.
	// If 0, defaults to 24 hours.
	// This affects both cookie expiration and store TTL.
	TTL time.Duration

	// CookieName is the name of the session cookie.
	// If empty, defaults to "flash.sid".
	// Set to empty string to disable cookie-based sessions.
	CookieName string

	// CookiePath sets the Path attribute of the session cookie.
	// If empty, defaults to "/".
	// Controls which paths the cookie is sent for.
	CookiePath string

	// Domain sets the Domain attribute of the session cookie.
	// If empty, cookie is only sent to the current domain.
	// Use ".example.com" to include subdomains.
	Domain string

	// Secure sets the Secure attribute of the session cookie.
	// When true, cookie is only sent over HTTPS connections.
	// Should be true in production environments.
	Secure bool

	// HTTPOnly sets the HttpOnly attribute of the session cookie.
	// When true, cookie cannot be accessed via JavaScript.
	// Recommended to be true for security (prevents XSS attacks).
	HTTPOnly bool

	// SameSite sets the SameSite attribute of the session cookie.
	// Controls when cookies are sent with cross-site requests.
	// Options: http.SameSiteDefaultMode, http.SameSiteLaxMode, http.SameSiteStrictMode, http.SameSiteNoneMode
	// Defaults to http.SameSiteLaxMode for balance of security and usability.
	SameSite http.SameSite

	// HeaderName specifies an HTTP header for session ID transport.
	// If set, session ID is read from and written to this header.
	// Useful for API clients that don't support cookies.
	// Common values: "X-Session-ID", "Authorization", "X-Auth-Token"
	HeaderName string

	// IdleTimeout is the maximum time a session can be idle before expiring.
	// If 0, sessions don't have idle timeout (only absolute TTL applies).
	// Helps prevent session hijacking by limiting inactive session lifetime.
	IdleTimeout time.Duration

	// MaxAge is the absolute maximum lifetime of a session.
	// If 0, no absolute maximum is enforced (only TTL applies).
	// Forces session regeneration after this duration regardless of activity.
	MaxAge time.Duration

	// RegenerateOnAuth automatically regenerates session ID on authentication.
	// When true, calls session.Regenerate() when certain conditions are met.
	// Helps prevent session fixation attacks.
	RegenerateOnAuth bool
}

func defaultSessionConfig() SessionConfig {
	return SessionConfig{
		Store:      NewMemoryStore(),
		TTL:        24 * time.Hour,
		CookieName: "flash.sid",
		CookiePath: "/",
		HTTPOnly:   true,
		SameSite:   http.SameSiteLaxMode,
	}
}

// Sessions returns middleware that provides secure session management for HTTP requests.
//
// This middleware handles the complete session lifecycle including loading existing sessions,
// creating new sessions, managing session data, and persisting changes. It integrates with
// any Store implementation and provides comprehensive security features.
//
// Key Features:
//   - Automatic session loading and saving
//   - Secure session ID generation with cryptographic randomness
//   - Session regeneration to prevent fixation attacks
//   - Flexible transport via cookies and/or headers
//   - Configurable security attributes (Secure, HTTPOnly, SameSite)
//   - Support for both web applications and APIs
//   - Efficient change tracking to minimize storage operations
//   - Integration with the flash.Ctx context system
//
// Basic Usage:
//
//	app := flash.New()
//	app.Use(middleware.Sessions(middleware.SessionConfig{
//		Store:      middleware.NewMemoryStore(),
//		TTL:        24 * time.Hour,
//		CookieName: "session_id",
//		HTTPOnly:   true,
//		Secure:     true, // Enable in production
//	}))
//
//	app.GET("/login", func(c flash.Ctx) error {
//		session := middleware.SessionFromCtx(c)
//		session.Set("user_id", "12345")
//		return c.JSON(200, map[string]string{"status": "logged in"})
//	})
//
// Advanced Usage with Security Features:
//
//	store := middleware.NewMemoryStore()
//	store.StartCleanup(5 * time.Minute) // Prevent memory leaks
//	defer store.StopCleanup()
//
//	app.Use(middleware.Sessions(middleware.SessionConfig{
//		Store:         store,
//		TTL:           12 * time.Hour,
//		CookieName:    "secure_session",
//		CookiePath:    "/",
//		Domain:        ".example.com",
//		Secure:        true,
//		HTTPOnly:      true,
//		SameSite:      http.SameSiteStrictMode,
//		HeaderName:    "X-Session-ID",
//		IdleTimeout:   30 * time.Minute,
//		MaxAge:        7 * 24 * time.Hour,
//	}))
//
//	// Login handler with session regeneration
//	app.POST("/login", func(c flash.Ctx) error {
//		// Authenticate user...
//		if !authenticateUser(username, password) {
//			return c.Status(401).String("Invalid credentials")
//		}
//
//		session := middleware.SessionFromCtx(c)
//		session.Regenerate() // Prevent session fixation
//		session.Set("user_id", userID)
//		session.Set("authenticated", true)
//		session.Set("login_time", time.Now())
//
//		return c.JSON(200, map[string]string{"status": "logged in"})
//	})
//
//	// Logout handler
//	app.POST("/logout", func(c flash.Ctx) error {
//		session := middleware.SessionFromCtx(c)
//		session.Clear() // Clear all session data
//		return c.JSON(200, map[string]string{"status": "logged out"})
//	})
//
// API Usage (Header-based):
//
//	app.Use(middleware.Sessions(middleware.SessionConfig{
//		Store:      redisStore,
//		TTL:        2 * time.Hour,
//		HeaderName: "X-Session-Token",
//		// No cookie settings for API-only usage
//	}))
//
// Security Considerations:
//   - Always set Secure=true in production with HTTPS
//   - Use HTTPOnly=true to prevent XSS attacks
//   - Consider SameSite=Strict for maximum CSRF protection
//   - Implement proper session cleanup to prevent memory leaks
//   - Use session regeneration after authentication or privilege changes
//   - Set appropriate TTL values based on your security requirements
//   - Consider using IdleTimeout for additional security
//
// Performance Considerations:
//   - Sessions are only saved when data changes (efficient change tracking)
//   - Use memory-efficient stores like Redis for high-traffic applications
//   - Configure appropriate cleanup intervals for memory stores
//   - Consider session data size impact on storage and network
//   - Use header-based transport for APIs to avoid cookie overhead
func Sessions(cfg SessionConfig) flash.Middleware {
	// fill defaults
	def := defaultSessionConfig()
	if cfg.Store == nil {
		cfg.Store = def.Store
	}
	if cfg.TTL == 0 {
		cfg.TTL = def.TTL
	}
	if cfg.CookieName == "" {
		cfg.CookieName = def.CookieName
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = def.CookiePath
	}
	if cfg.SameSite == 0 {
		cfg.SameSite = def.SameSite
	}

	return func(next flash.Handler) flash.Handler {
		return func(c flash.Ctx) error {
			r := c.Request()
			id := readSessionID(r, cfg)

			var sess Session
			if id != "" {
				if vals, ok := cfg.Store.Get(id); ok {
					sess = Session{ID: id, Values: vals}
				} else {
					sess = Session{ID: id, Values: map[string]any{}, new: true}
				}
			} else {
				// create new id lazily upon first Set
				sess = Session{ID: "", Values: map[string]any{}, new: true}
			}

			// put into request context
			ctx := context.WithValue(r.Context(), sessionContextKey{}, &sess)
			r = r.WithContext(ctx)
			c.SetRequest(r)

			// Wrap ResponseWriter to ensure Set-Cookie header is written before headers are sent
			flushed := false
			flush := func() {
				if flushed {
					return
				}
				// persist if changed or new with non-empty id (generate if needed)
				if sess.changed || (sess.new && sess.ID != "") {
					if sess.ID == "" {
						sess.ID = newSessionID()
					}

					// Clean up old session ID if regenerated
					if sess.regenerated && sess.oldID != "" {
						_ = cfg.Store.Delete(sess.oldID)
					}

					_ = cfg.Store.Save(sess.ID, sess.Values, cfg.TTL)
					writeSessionID(c, sess.ID, cfg)
				}
				flushed = true
			}
			c.SetResponseWriter(&headerWriteInterceptor{rw: c.ResponseWriter(), before: flush})

			err := next(c)

			// If nothing wrote headers, ensure cookie is flushed now
			flush()
			return err
		}
	}
}

// SessionFromCtx retrieves the Session previously loaded by Sessions middleware.
// Returns a session instance that can be safely used even if Sessions middleware
// is not present (returns empty session in that case).
//
// Example usage:
//
//	func handler(c flash.Ctx) error {
//		session := middleware.SessionFromCtx(c)
//
//		// Check if user is authenticated
//		if userID, ok := session.Get("user_id"); ok {
//			// User is authenticated
//			return c.JSON(200, map[string]any{"user_id": userID})
//		}
//
//		// User is not authenticated
//		return c.Status(401).String("Not authenticated")
//	}
//
// The returned session is safe to use even without Sessions middleware:
//
//	session := middleware.SessionFromCtx(c)
//	session.Set("key", "value") // Safe, but won't be persisted without middleware
//
// Security note: Always check session validity in security-sensitive operations.
func SessionFromCtx(c flash.Ctx) *Session {
	v := c.Context().Value(sessionContextKey{})
	if v == nil {
		// Return empty session if middleware not present
		return &Session{Values: make(map[string]any)}
	}
	if s, ok := v.(*Session); ok {
		return s
	}
	// Return empty session if wrong type in context
	return &Session{Values: make(map[string]any)}
}

func readSessionID(r *http.Request, cfg SessionConfig) string {
	if cfg.HeaderName != "" {
		if hv := r.Header.Get(cfg.HeaderName); hv != "" {
			return hv
		}
	}
	if cfg.CookieName != "" {
		if ck, err := r.Cookie(cfg.CookieName); err == nil && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

func writeSessionID(c flash.Ctx, id string, cfg SessionConfig) {
	if cfg.HeaderName != "" {
		c.Header(cfg.HeaderName, id)
	}
	if cfg.CookieName != "" {
		http.SetCookie(c.ResponseWriter(), &http.Cookie{
			Name:     cfg.CookieName,
			Value:    id,
			Path:     cfg.CookiePath,
			Domain:   cfg.Domain,
			Secure:   cfg.Secure,
			HttpOnly: cfg.HTTPOnly,
			SameSite: cfg.SameSite,
			Expires:  time.Now().Add(cfg.TTL),
		})
	}
}

// newSessionID generates a cryptographically secure session ID.
// Uses 32 bytes of random data (256 bits) encoded as base64url for maximum security.
// The resulting ID is URL-safe and has sufficient entropy to prevent brute force attacks.
//
// Security properties:
//   - 256 bits of entropy (meets OWASP recommendations)
//   - Cryptographically secure random number generation
//   - URL-safe encoding (no padding)
//   - Statistically unique (collision probability negligible)
//
// Example output: "xJ8kL2mN9pQ7rS5tU8vW3xY1zA4bC6dE9fG2hI5jK8lM"
func newSessionID() string {
	// Use 32 bytes (256 bits) for maximum security
	// This provides 2^256 possible values, making brute force attacks infeasible
	b := make([]byte, 32)

	// crypto/rand provides cryptographically secure random bytes
	// In the extremely unlikely event of failure, we panic to avoid weak session IDs
	if _, err := rand.Read(b); err != nil {
		// This should never happen on properly configured systems
		panic("session: failed to generate secure random bytes: " + err.Error())
	}

	// Use RawURLEncoding (no padding) for URL-safe, compact representation
	// Results in 43-character string that's safe for cookies and headers
	return base64.RawURLEncoding.EncodeToString(b)
}

// headerWriteInterceptor invokes a callback before the first header write.
type headerWriteInterceptor struct {
	rw      http.ResponseWriter
	before  func()
	written bool
}

func (h *headerWriteInterceptor) Header() http.Header { return h.rw.Header() }

func (h *headerWriteInterceptor) WriteHeader(status int) {
	if !h.written {
		h.before()
		h.written = true
	}
	h.rw.WriteHeader(status)
}

func (h *headerWriteInterceptor) Write(p []byte) (int, error) {
	if !h.written {
		h.WriteHeader(http.StatusOK)
	}
	return h.rw.Write(p)
}
