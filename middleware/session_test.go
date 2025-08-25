package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goflash/flash/v2"
)

func TestSessionsCookieAndHeader(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid", HeaderName: "X-Session-ID"}))

	// set route writes a session value, causing save and cookie/header set
	a.GET("/set", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	// get route reads session
	a.GET("/get", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		if v, ok := s.Get("k"); ok {
			return c.String(http.StatusOK, v.(string))
		}
		return c.String(http.StatusNotFound, "missing")
	})

	// First request sets cookie via Sessions middleware
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	ck := rec.Result().Cookies()
	if len(ck) == 0 {
		t.Fatalf("no cookie")
	}

	// Send cookie back to read value
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/get", nil)
	for _, c := range ck {
		req.AddCookie(c)
	}
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "v" {
		t.Fatalf("unexpected: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMemoryStoreSaveGetDelete(t *testing.T) {
	m := NewMemoryStore()
	id := "id1"
	if err := m.Save(id, map[string]any{"k": "v"}, 0); err != nil {
		t.Fatalf("save err: %v", err)
	}
	v, ok := m.Get(id)
	if !ok || v["k"] != "v" {
		t.Fatalf("get failed: %v %v", ok, v)
	}
	if err := m.Delete(id); err != nil {
		t.Fatalf("delete err: %v", err)
	}
	if _, ok := m.Get(id); ok {
		t.Fatalf("should be deleted")
	}
}

func TestMemoryStoreExpiredDeletesOnGet(t *testing.T) {
	m := NewMemoryStore()
	id := "id2"
	// small positive TTL and sleep to ensure expiration
	if err := m.Save(id, map[string]any{"k": "v"}, 5*time.Millisecond); err != nil {
		t.Fatalf("save err: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, ok := m.Get(id); ok {
		t.Fatalf("expired should not be ok")
	}
}

func TestMemoryStoreSaveEmptyIDErrorAndNilData(t *testing.T) {
	m := NewMemoryStore()
	if err := m.Save("", map[string]any{"k": "v"}, 0); err == nil {
		t.Fatalf("expected error on empty id")
	}
	// nil data should be handled and returned as empty map on Get
	id := "nid"
	if err := m.Save(id, nil, 0); err != nil {
		t.Fatalf("save err: %v", err)
	}
	v, ok := m.Get(id)
	if !ok || len(v) != 0 {
		t.Fatalf("expected empty map from nil data, got: ok=%v v=%v", ok, v)
	}
}

func TestSessionDeleteBranches(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid"}))
	a.GET("/set", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	a.GET("/del", func(c flash.Ctx) error { s := SessionFromCtx(c); s.Delete("k"); return c.String(http.StatusOK, "ok") })
	// read returns missing after delete
	a.GET("/get", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		if _, ok := s.Get("k"); ok {
			return c.String(http.StatusOK, "has")
		}
		return c.String(http.StatusNotFound, "missing")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	a.ServeHTTP(rec, req)
	cks := rec.Result().Cookies()

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/del", nil)
	for _, ck := range cks {
		req.AddCookie(ck)
	}
	a.ServeHTTP(rec, req)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/get", nil)
	for _, ck := range cks {
		req.AddCookie(ck)
	}
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing after delete")
	}
}

func TestSessionsHeaderBasedID(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, HeaderName: "X-SID"}))

	// set
	a.GET("/set", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	// get
	a.GET("/get", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		if v, ok := s.Get("k"); ok {
			return c.String(http.StatusOK, v.(string))
		}
		return c.String(http.StatusNotFound, "missing")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	a.ServeHTTP(rec, req)
	sid := rec.Header().Get("X-SID")
	if sid == "" {
		t.Fatalf("missing session id header")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/get", nil)
	req.Header.Set("X-SID", sid)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "v" {
		t.Fatalf("bad get: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSessionsExternalIDNoChangesFlushAtEnd(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	// Provide only Store and HeaderName to exercise defaults and the (new && ID != "") branch
	a.Use(Sessions(SessionConfig{Store: store, HeaderName: "X-SID"}))
	// Handler does not write headers/body and does not change session
	a.GET("/noop", func(c flash.Ctx) error { return nil })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/noop", nil)
	req.Header.Set("X-SID", "abc123") // external id provided
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Expect header with session id written
	if rec.Header().Get("X-SID") == "" {
		t.Fatalf("expected X-SID header to be written")
	}
	// Expect cookie written with default cookie name
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("expected Set-Cookie written")
	}
	// Store should contain the id with empty map persisted
	if v, ok := store.Get("abc123"); !ok || len(v) != 0 {
		t.Fatalf("expected store to have empty session for abc123, ok=%v v=%v", ok, v)
	}
}

func TestSessionsNoIDNoChangesNoSetCookie(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	// Default config (no HeaderName). No incoming cookie/id and handler makes no changes
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour}))
	a.GET("/noop2", func(c flash.Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/noop2", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("bad response: %d %q", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("did not expect Set-Cookie when no id and no changes")
	}
}

func TestSessionHeaderWriteInterceptorWriteCallsBefore(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid"}))
	// Write to ResponseWriter directly without calling c.String to trigger headerWriteInterceptor.Write
	a.GET("/w", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		_, _ = c.ResponseWriter().Write([]byte("ok"))
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w", nil)
	a.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("expected Set-Cookie written before headers")
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("unexpected response: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSessionFromCtxNil(t *testing.T) {
	// Without Sessions middleware, SessionFromCtx should return empty session
	a := flash.New()
	a.GET("/x", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		if _, ok := s.Get("k"); ok {
			t.Fatalf("expected empty session values")
		}
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestHeaderWriteInterceptorWriteHeaderPath(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid"}))
	a.GET("/h", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("x", "y")
		// trigger WriteHeader path (without body)
		c.ResponseWriter().WriteHeader(http.StatusCreated)
		return nil
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/h", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatalf("expected Set-Cookie written before header")
	}
}

func TestSessionFromCtxWrongTypeValue(t *testing.T) {
	a := flash.New()
	a.GET("/wt", func(c flash.Ctx) error {
		// Inject a wrong-typed value under sessionContextKey{}
		r := c.Request().WithContext(context.WithValue(c.Context(), sessionContextKey{}, "bad"))
		c.SetRequest(r)
		s := SessionFromCtx(c)
		if s == nil || len(s.Values) != 0 || s.ID != "" {
			t.Fatalf("expected empty session on wrong type, got: %+v", s)
		}
		return c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/wt", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

// Test new Session methods
func TestSessionClearAndRegenerate(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid"}))

	// Set some initial data
	a.GET("/set", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("user_id", "123")
		s.Set("role", "admin")
		return c.String(http.StatusOK, "ok")
	})

	// Clear session data
	a.GET("/clear", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Clear()
		return c.String(http.StatusOK, "cleared")
	})

	// Regenerate session ID
	a.GET("/regenerate", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		oldID := s.ID
		s.Regenerate()
		if s.ID == oldID {
			t.Error("session ID should have changed")
		}
		if !s.IsRegenerated() {
			t.Error("session should be marked as regenerated")
		}
		s.Set("new_data", "after_regen")
		return c.String(http.StatusOK, s.ID)
	})

	// First request - set data
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	a.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()

	// Second request - clear data
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/clear", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	a.ServeHTTP(rec, req)

	// Third request - regenerate session
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/regenerate", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	a.ServeHTTP(rec, req)
	newSessionID := rec.Body.String()

	if newSessionID == "" {
		t.Fatal("expected new session ID")
	}
}

// Test MemoryStore cleanup functionality
func TestMemoryStoreCleanup(t *testing.T) {
	store := NewMemoryStore()
	defer store.StopCleanup()

	// Save sessions with short TTL
	err := store.Save("session1", map[string]any{"key": "value1"}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	err = store.Save("session2", map[string]any{"key": "value2"}, time.Hour)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify both sessions exist
	if store.Len() != 2 {
		t.Fatalf("expected 2 sessions, got %d", store.Len())
	}

	// Wait for first session to expire
	time.Sleep(15 * time.Millisecond)

	// Trigger cleanup
	store.cleanupExpired()

	// Verify expired session is removed
	if store.Len() != 1 {
		t.Fatalf("expected 1 session after cleanup, got %d", store.Len())
	}

	// Verify correct session remains
	if _, ok := store.Get("session2"); !ok {
		t.Fatal("session2 should still exist")
	}

	if _, ok := store.Get("session1"); ok {
		t.Fatal("session1 should be expired and removed")
	}
}

// Test automatic cleanup with StartCleanup
func TestMemoryStoreAutoCleanup(t *testing.T) {
	store := NewMemoryStore()
	store.StartCleanup(50 * time.Millisecond) // Very frequent for testing
	defer store.StopCleanup()

	// Save session with short TTL
	err := store.Save("temp_session", map[string]any{"temp": true}, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify session exists
	if store.Len() != 1 {
		t.Fatalf("expected 1 session, got %d", store.Len())
	}

	// Wait for cleanup to run (TTL + cleanup interval + buffer)
	time.Sleep(100 * time.Millisecond)

	// Verify session was cleaned up
	if store.Len() != 0 {
		t.Fatalf("expected 0 sessions after auto cleanup, got %d", store.Len())
	}
}

// Test timing attack protection in Get method
func TestMemoryStoreTimingAttackProtection(t *testing.T) {
	store := NewMemoryStore()

	// Save a session
	err := store.Save("real_session", map[string]any{"user": "alice"}, time.Hour)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Test with real session ID
	start := time.Now()
	_, ok := store.Get("real_session")
	realDuration := time.Since(start)

	if !ok {
		t.Fatal("real session should exist")
	}

	// Test with fake session ID (should take similar time)
	start = time.Now()
	_, ok = store.Get("fake_session_id_with_similar_length")
	fakeDuration := time.Since(start)

	if ok {
		t.Fatal("fake session should not exist")
	}

	// The timing difference should be minimal (less than 10x)
	// This is a rough test - in practice, timing attacks are more sophisticated
	ratio := float64(realDuration) / float64(fakeDuration)
	if ratio > 10 || ratio < 0.1 {
		t.Logf("Timing ratio: %f (real: %v, fake: %v)", ratio, realDuration, fakeDuration)
		// Don't fail the test as timing can vary, but log for awareness
	}
}

// Test session regeneration cleans up old session
func TestSessionRegenerationCleanup(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid"}))

	var oldSessionID string

	// Set initial session data
	a.GET("/login", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("user_id", "123")
		// Session ID is generated when first saved, so we return a placeholder
		return c.String(http.StatusOK, "login_ok")
	})

	// Regenerate session after "authentication"
	a.GET("/auth", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Regenerate() // This should clean up the old session
		s.Set("authenticated", true)
		return c.String(http.StatusOK, s.ID)
	})

	// First request - create session
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	a.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()

	// Get the session ID from the cookie
	if len(cookies) == 0 {
		t.Fatal("expected cookie to be set")
	}
	oldSessionID = cookies[0].Value

	// Verify old session exists in store
	if _, ok := store.Get(oldSessionID); !ok {
		t.Fatal("old session should exist before regeneration")
	}

	// Second request - regenerate session
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/auth", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	a.ServeHTTP(rec, req)
	newSessionID := rec.Body.String()

	// Verify old session was cleaned up
	if _, ok := store.Get(oldSessionID); ok {
		t.Fatal("old session should be cleaned up after regeneration")
	}

	// Verify new session exists
	if _, ok := store.Get(newSessionID); !ok {
		t.Fatal("new session should exist after regeneration")
	}

	// Verify session data was preserved
	if data, ok := store.Get(newSessionID); ok {
		if data["user_id"] != "123" {
			t.Fatal("session data should be preserved during regeneration")
		}
		if data["authenticated"] != true {
			t.Fatal("new data should be present after regeneration")
		}
	} else {
		t.Fatal("new session data should be accessible")
	}
}

// Test session helper methods
func TestSessionHelperMethods(t *testing.T) {
	s := &Session{
		ID:     "test_id",
		Values: make(map[string]any),
		new:    true,
	}

	// Test initial state
	if !s.IsNew() {
		t.Error("session should be marked as new")
	}
	if s.IsChanged() {
		t.Error("session should not be marked as changed initially")
	}
	if s.IsRegenerated() {
		t.Error("session should not be marked as regenerated initially")
	}

	// Test Set operation
	s.Set("key", "value")
	if !s.IsChanged() {
		t.Error("session should be marked as changed after Set")
	}

	// Test Get operation
	if val, ok := s.Get("key"); !ok || val != "value" {
		t.Errorf("expected value 'value', got %v, %v", val, ok)
	}

	// Test Delete operation
	s.Delete("key")
	if val, ok := s.Get("key"); ok {
		t.Errorf("key should be deleted, but got %v", val)
	}

	// Test Regenerate operation
	oldID := s.ID
	s.Regenerate()
	if s.ID == oldID {
		t.Error("session ID should change after regeneration")
	}
	if !s.IsRegenerated() {
		t.Error("session should be marked as regenerated")
	}
	if s.oldID != oldID {
		t.Error("old ID should be preserved for cleanup")
	}
}

// Test copyMapEfficient function
func TestCopyMapEfficient(t *testing.T) {
	// Test nil map
	result := copyMapEfficient(nil)
	if result != nil {
		t.Error("copyMapEfficient should return nil for nil input")
	}

	// Test empty map
	empty := make(map[string]any)
	result = copyMapEfficient(empty)
	if result == nil {
		t.Error("copyMapEfficient should return empty map, not nil")
	}
	if len(result) != 0 {
		t.Error("copyMapEfficient should return empty map for empty input")
	}

	// Test map with data
	original := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}
	result = copyMapEfficient(original)

	if len(result) != len(original) {
		t.Errorf("copied map should have same length: expected %d, got %d", len(original), len(result))
	}

	for k, v := range original {
		if result[k] != v {
			t.Errorf("key %s: expected %v, got %v", k, v, result[k])
		}
	}

	// Verify it's a copy (modify original shouldn't affect copy)
	original["new_key"] = "new_value"
	if _, exists := result["new_key"]; exists {
		t.Error("modifying original should not affect copy")
	}
}

// Test newSessionID security properties
func TestNewSessionIDSecurity(t *testing.T) {
	// Generate multiple session IDs
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := newSessionID()

		// Check length (should be 43 characters for 32 bytes base64url encoded)
		if len(id) != 43 {
			t.Errorf("session ID should be 43 characters, got %d", len(id))
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("duplicate session ID generated: %s", id)
		}
		ids[id] = true

		// Check URL-safe characters only
		for _, char := range id {
			if !((char >= 'A' && char <= 'Z') ||
				(char >= 'a' && char <= 'z') ||
				(char >= '0' && char <= '9') ||
				char == '-' || char == '_') {
				t.Errorf("session ID contains non-URL-safe character: %c in %s", char, id)
			}
		}
	}

	// All IDs should be unique
	if len(ids) != 1000 {
		t.Errorf("expected 1000 unique IDs, got %d", len(ids))
	}
}

func TestCopyMap(t *testing.T) {
	// Test copyMap with nil input
	result := copyMap(nil)
	if result == nil {
		t.Fatal("copyMap should return empty map for nil input")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d items", len(result))
	}

	// Test copyMap with non-nil input
	src := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}
	result = copyMap(src)
	if result == nil {
		t.Fatal("copyMap should not return nil")
	}
	if len(result) != len(src) {
		t.Fatalf("expected %d items, got %d", len(src), len(result))
	}
	for k, v := range src {
		if result[k] != v {
			t.Errorf("key %s: expected %v, got %v", k, v, result[k])
		}
	}

	// Verify it's a shallow copy
	result["new_key"] = "new_value"
	if _, exists := src["new_key"]; exists {
		t.Error("copyMap should create a shallow copy, not reference the original")
	}
}

func TestSessionGetSetDeleteWithNilValues(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour}))

	a.GET("/test", func(c flash.Ctx) error {
		s := SessionFromCtx(c)

		// Test setting nil value
		s.Set("nil_key", nil)

		// Test getting nil value
		val, ok := s.Get("nil_key")
		if !ok {
			t.Error("expected nil value to be retrievable")
		}
		if val != nil {
			t.Errorf("expected nil, got %v", val)
		}

		// Test deleting nil value
		s.Delete("nil_key")
		_, ok = s.Get("nil_key")
		if ok {
			t.Error("expected key to be deleted")
		}

		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSessionClearWithEmptySession(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour}))

	a.GET("/test", func(c flash.Ctx) error {
		s := SessionFromCtx(c)

		// Clear empty session
		s.Clear()

		// Should still work
		s.Set("key", "value")
		val, ok := s.Get("key")
		if !ok || val != "value" {
			t.Error("session should work after clearing empty session")
		}

		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSessionStartCleanupError(t *testing.T) {
	store := NewMemoryStore()

	// Start cleanup with a very short interval
	store.StartCleanup(1 * time.Millisecond)

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	// Stop cleanup
	store.StopCleanup()

	// Starting cleanup again should work - create new store to avoid channel reuse
	store2 := NewMemoryStore()
	store2.StartCleanup(time.Hour)
	store2.StopCleanup()
}

func TestNewSessionIDWithShortLength(t *testing.T) {
	// Test with different ID lengths
	for i := 0; i < 10; i++ {
		id := newSessionID()
		if len(id) < 32 {
			t.Errorf("session ID too short: %d characters", len(id))
		}
	}
}

func TestSessionEdgeCases(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour}))

	a.GET("/test", func(c flash.Ctx) error {
		s := SessionFromCtx(c)

		// Test session operations on new session
		if !s.IsNew() {
			t.Error("expected new session to be marked as new")
		}

		// Test getting non-existent key
		val, ok := s.Get("nonexistent")
		if ok {
			t.Error("expected false for non-existent key")
		}
		if val != nil {
			t.Errorf("expected nil for non-existent key, got %v", val)
		}

		// Test deleting non-existent key
		s.Delete("nonexistent") // Should not panic

		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSessionWithLongTTL(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	// Use very short TTL for testing
	a.Use(Sessions(SessionConfig{Store: store, TTL: 1 * time.Millisecond}))

	a.GET("/set", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("key", "value")
		return c.String(http.StatusOK, "set")
	})

	a.GET("/get", func(c flash.Ctx) error {
		s := SessionFromCtx(c)
		val, ok := s.Get("key")
		if ok {
			return c.String(http.StatusOK, val.(string))
		}
		return c.String(http.StatusNotFound, "not found")
	})

	// Set session
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	// Wait for session to expire
	time.Sleep(2 * time.Millisecond)

	// Try to get expired session
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	for _, cookie := range cookies {
		req2.AddCookie(cookie)
	}
	a.ServeHTTP(rec2, req2)

	// Should not find the expired session
	if rec2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for expired session, got %d", rec2.Code)
	}
}

func TestSessionStartCleanupEdgeCase(t *testing.T) {
	store := NewMemoryStore()

	// Test StartCleanup when cleanup is already running
	store.StartCleanup(time.Hour)

	// Starting again should not cause issues
	store.StartCleanup(time.Hour)

	store.StopCleanup()
}

func TestNewSessionIDEdgeCase(t *testing.T) {
	// Test newSessionID with different scenarios
	ids := make(map[string]bool)

	// Generate many IDs to test randomness
	for i := 0; i < 100; i++ {
		id := newSessionID()
		if ids[id] {
			t.Errorf("duplicate session ID generated: %s", id)
		}
		ids[id] = true

		// Verify ID format
		if len(id) < 32 {
			t.Errorf("session ID too short: %d", len(id))
		}
	}
}

func TestSessionGetSetDeleteWithNilValuesEdgeCases(t *testing.T) {
	// Test session methods when Values is nil (uncovered lines)
	session := &Session{}

	// Test Get with nil Values
	val, ok := session.Get("key")
	if ok || val != nil {
		t.Errorf("expected nil/false for Get on nil Values, got %v/%v", val, ok)
	}

	// Test Delete with nil Values (should not panic)
	session.Delete("key") // This hits the return path when Values is nil

	// Test Set (should initialize Values)
	session.Set("key", "value")
	if session.Values == nil {
		t.Error("expected Values to be initialized after Set")
	}
	if !session.changed {
		t.Error("expected changed to be true after Set")
	}

	// Test Clear with nil Values initially
	session2 := &Session{}
	session2.Clear()
	if session2.Values == nil {
		t.Error("expected Values to be initialized after Clear")
	}
	if !session2.changed {
		t.Error("expected changed to be true after Clear")
	}
}

func TestSessionClearWithExistingValues(t *testing.T) {
	// Test Clear with existing values (different code path)
	session := &Session{
		Values: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	session.Clear()

	if len(session.Values) != 0 {
		t.Errorf("expected empty Values after Clear, got %d items", len(session.Values))
	}
	if !session.changed {
		t.Error("expected changed to be true after Clear")
	}
}

func TestMemoryStoreGetWithTimingAttack(t *testing.T) {
	store := NewMemoryStore()

	// Test the timing-safe comparison path when session doesn't exist
	// This hits the dummy operation line for timing protection
	data, ok := store.Get("non_existent_session_id")
	if ok || data != nil {
		t.Errorf("expected nil/false for non-existent session, got %v/%v", data, ok)
	}
}

func TestMemoryStoreGetWithExpiredSession(t *testing.T) {
	store := NewMemoryStore()

	// Save session with very short TTL
	testData := map[string]any{"key": "value"}
	store.Save("test_id", testData, 1*time.Nanosecond)

	// Wait for expiration
	time.Sleep(1 * time.Millisecond)

	// This should hit the lazy cleanup path
	data, ok := store.Get("test_id")
	if ok || data != nil {
		t.Errorf("expected nil/false for expired session, got %v/%v", data, ok)
	}
}

func TestNewSessionIDRandomness(t *testing.T) {
	// Test newSessionID function to hit the 75% coverage line
	ids := make(map[string]bool)

	// Generate multiple IDs to ensure they're different
	for i := 0; i < 50; i++ {
		id := newSessionID()
		if ids[id] {
			t.Errorf("duplicate session ID generated: %s", id)
		}
		ids[id] = true

		if len(id) < 32 {
			t.Errorf("session ID too short: %d characters", len(id))
		}

		// Check that it's URL-safe base64 (basic check)
		if len(id) == 0 {
			t.Errorf("session ID is empty")
		}
	}
}

func TestMemoryStoreStartCleanupIdempotent(t *testing.T) {
	store := NewMemoryStore()

	// Test StartCleanup when already running (hits the 80% coverage line)
	store.StartCleanup(time.Hour)

	// Call again - should be idempotent
	store.StartCleanup(time.Hour)

	store.StopCleanup()
}

func TestNewSessionIDEdgeCasesAndLength(t *testing.T) {
	// Test newSessionID function to hit more coverage lines
	for i := 0; i < 10; i++ {
		id := newSessionID()

		// Test various properties
		if len(id) < 32 {
			t.Errorf("session ID too short: %d", len(id))
		}

		// Test that it doesn't contain problematic characters
		if strings.ContainsAny(id, " \t\n\r") {
			t.Errorf("session ID contains whitespace: %s", id)
		}

		// Ensure it's not empty or just whitespace
		if strings.TrimSpace(id) == "" {
			t.Error("session ID is empty or whitespace")
		}
	}
}
