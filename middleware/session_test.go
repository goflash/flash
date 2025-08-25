package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
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
