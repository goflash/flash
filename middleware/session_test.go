package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goflash/flash"
)

func TestSessionsCookieAndHeader(t *testing.T) {
	store := NewMemoryStore()
	a := flash.New()
	a.Use(Sessions(SessionConfig{Store: store, TTL: time.Hour, CookieName: "sid", HeaderName: "X-Session-ID"}))

	// set route writes a session value, causing save and cookie/header set
	a.GET("/set", func(c *flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	// get route reads session
	a.GET("/get", func(c *flash.Ctx) error {
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
	a.GET("/set", func(c *flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	a.GET("/del", func(c *flash.Ctx) error { s := SessionFromCtx(c); s.Delete("k"); return c.String(http.StatusOK, "ok") })
	// read returns missing after delete
	a.GET("/get", func(c *flash.Ctx) error {
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
	a.GET("/set", func(c *flash.Ctx) error {
		s := SessionFromCtx(c)
		s.Set("k", "v")
		return c.String(http.StatusOK, "ok")
	})
	// get
	a.GET("/get", func(c *flash.Ctx) error {
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
	a.GET("/noop", func(c *flash.Ctx) error { return nil })

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
	a.GET("/noop2", func(c *flash.Ctx) error { return c.String(http.StatusOK, "ok") })

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
	a.GET("/w", func(c *flash.Ctx) error {
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
	a.GET("/x", func(c *flash.Ctx) error {
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
	a.GET("/h", func(c *flash.Ctx) error {
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
	a.GET("/wt", func(c *flash.Ctx) error {
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
