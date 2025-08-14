package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/goflash/flash"
)

type sessionContextKey struct{}

// Store abstracts session persistence for the session middleware.
// Implementations must provide Get, Save, and Delete for session data by ID.
type Store interface {
	Get(id string) (map[string]any, bool)
	Save(id string, data map[string]any, ttl time.Duration) error
	Delete(id string) error
}

// MemoryStore is a simple in-memory session store with TTL.
// Not suitable for production or multi-process deployments.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]entry
}

type entry struct {
	v   map[string]any
	exp time.Time
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{data: make(map[string]entry)} }

func (m *MemoryStore) Get(id string) (map[string]any, bool) {
	m.mu.RLock()
	e, ok := m.data[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		_ = m.Delete(id)
		return nil, false
	}
	return copyMap(e.v), true
}

func (m *MemoryStore) Save(id string, data map[string]any, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty session id")
	}
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.data[id] = entry{v: copyMap(data), exp: exp}
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Delete(id string) error {
	m.mu.Lock()
	delete(m.data, id)
	m.mu.Unlock()
	return nil
}

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

// Session represents the per-request view of a session.
type Session struct {
	ID      string
	Values  map[string]any
	changed bool
	new     bool
}

func (s *Session) Get(key string) (any, bool) { v, ok := s.Values[key]; return v, ok }
func (s *Session) Set(key string, v any)      { s.Values[key] = v; s.changed = true }
func (s *Session) Delete(key string)          { delete(s.Values, key); s.changed = true }

// SessionConfig configures the session middleware.
type SessionConfig struct {
	Store      Store
	TTL        time.Duration
	CookieName string
	CookiePath string
	Domain     string
	Secure     bool
	HTTPOnly   bool
	SameSite   http.SameSite
	HeaderName string // if set, read/write session id via header as well
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

// Sessions returns middleware that loads/saves a session around each request.
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
		return func(c *flash.Ctx) error {
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
func SessionFromCtx(c *flash.Ctx) *Session {
	v := c.Context().Value(sessionContextKey{})
	if v == nil {
		return &Session{Values: map[string]any{}}
	}
	if s, ok := v.(*Session); ok {
		return s
	}
	return &Session{Values: map[string]any{}}
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

func writeSessionID(c *flash.Ctx, id string, cfg SessionConfig) {
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

func newSessionID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
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
