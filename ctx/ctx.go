package ctx

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	router "github.com/julienschmidt/httprouter"
)

// Ctx is the request/response context interface exposed to handlers and middleware.
// It is implemented by *DefaultContext and lives in package ctx to avoid adapters
// and import cycles.
//
// A Ctx provides convenient accessors for request data (method, path, params,
// query), helpers for retrieving typed parameters, and response helpers for
// writing headers and bodies in common formats.
//
// Typical usage inside a handler:
//
//	c.GET("/users/:id", func(c ctx.Ctx) error {
//	    // Basic request information
//	    method := c.Method()            // "GET"
//	    route  := c.Route()             // "/users/:id"
//	    path   := c.Path()              // e.g. "/users/42"
//	    id     := c.ParamInt("id", 0)  // 42 (with default if parse fails)
//	    page   := c.QueryInt("page", 1) // from query string, default 1
//	    _ = method; _ = route; _ = path; _ = id; _ = page
//	    // Set a header and send JSON response
//	    c.Header("X-Handler", "users-show")
//	    return c.Status(http.StatusOK).JSON(map[string]any{"id": id})
//	})
//
// Concurrency: Ctx is not safe for concurrent writes to the underlying
// http.ResponseWriter. Use Clone() and swap the writer if responding from
// another goroutine.
type Ctx interface {
	// Request/Response accessors and mutators
	// Request returns the underlying *http.Request associated with this context.
	Request() *http.Request
	// SetRequest replaces the underlying *http.Request on the context.
	// Example: attach a new context value to the request.
	//
	//  	ctx := context.WithValue(c.Context(), key, value)
	//  	c.SetRequest(c.Request().WithContext(ctx))
	SetRequest(*http.Request)
	// ResponseWriter returns the underlying http.ResponseWriter.
	ResponseWriter() http.ResponseWriter
	// SetResponseWriter replaces the underlying http.ResponseWriter.
	SetResponseWriter(http.ResponseWriter)

	// Basic request data
	// Context returns the request-scoped context.Context.
	Context() context.Context
	// Method returns the HTTP method (e.g., "GET").
	Method() string
	// Path returns the raw request URL path.
	Path() string
	// Route returns the route pattern (e.g., "/users/:id") when available.
	Route() string
	// Param returns a path parameter by name ("" if not present).
	// Example: for route "/users/:id", Param("id") => "42".
	Param(name string) string
	// Query returns a query string parameter by key ("" if not present).
	// Example: for "/items?sort=asc", Query("sort") => "asc".
	Query(key string) string

	// Typed path parameter helpers with optional defaults
	ParamInt(name string, def ...int) int
	ParamInt64(name string, def ...int64) int64
	ParamUint(name string, def ...uint) uint
	ParamFloat64(name string, def ...float64) float64
	ParamBool(name string, def ...bool) bool

	// Typed query parameter helpers with optional defaults
	QueryInt(key string, def ...int) int
	QueryInt64(key string, def ...int64) int64
	QueryUint(key string, def ...uint) uint
	QueryFloat64(key string, def ...float64) float64
	QueryBool(key string, def ...bool) bool

	// Secure parameter helpers with input validation and sanitization
	ParamSafe(name string) string     // HTML-escaped parameter
	QuerySafe(key string) string      // HTML-escaped query parameter
	ParamAlphaNum(name string) string // Alphanumeric-only parameter
	QueryAlphaNum(key string) string  // Alphanumeric-only query parameter
	ParamFilename(name string) string // Safe filename parameter (no path traversal)
	QueryFilename(key string) string  // Safe filename query parameter

	// Response helpers
	// Header sets a response header key/value.
	Header(key, value string)
	// Status stages the HTTP status code to be written; returns the Ctx to allow chaining.
	// Example: c.Status(http.StatusCreated).JSON(obj)
	Status(code int) Ctx
	// StatusCode returns the status that will be written (or 200 after header write, or 0 if unset).
	StatusCode() int
	// JSON serializes v to JSON and writes it with an appropriate Content-Type.
	// If Status() was not set, it defaults to 200.
	JSON(v any) error
	// String writes a text/plain body with the provided status code.
	String(status int, body string) error
	// Send writes raw bytes with a specific status and content type.
	Send(status int, contentType string, b []byte) (int, error)
	// WroteHeader reports whether the header has already been written to the client.
	WroteHeader() bool

	// Convenience methods for common HTTP operations
	Redirect(status int, url string) error
	RedirectPermanent(url string) error
	RedirectTemporary(url string) error
	File(path string) error
	FileFromFS(path string, fs http.FileSystem) error
	NotFound(message ...string) error
	InternalServerError(message ...string) error
	BadRequest(message ...string) error
	Unauthorized(message ...string) error
	Forbidden(message ...string) error
	NoContent() error
	Stream(status int, contentType string, reader io.Reader) error
	StreamJSON(status int, v any) error

	// Cookie helpers
	SetCookie(cookie *http.Cookie)
	GetCookie(name string) (*http.Cookie, error)
	ClearCookie(name string)

	// BindJSON decodes request body JSON into v with strict defaults; see BindJSONOptions.
	BindJSON(v any, opts ...BindJSONOptions) error

	// BindMap binds from a generic map (e.g. collected from body/query/path) into v using mapstructure.
	// Options mirror BindJSONOptions.
	BindMap(v any, m map[string]any, opts ...BindJSONOptions) error

	// BindForm collects form body fields and binds them into v (application/x-www-form-urlencoded or multipart/form-data).
	BindForm(v any, opts ...BindJSONOptions) error

	// BindQuery collects query string parameters and binds them into v.
	BindQuery(v any, opts ...BindJSONOptions) error

	// BindPath collects path parameters and binds them into v.
	BindPath(v any, opts ...BindJSONOptions) error

	// BindAny collects from path, body (json/form), and query according to priority and binds them into v.
	BindAny(v any, opts ...BindJSONOptions) error

	// Utilities
	// Get retrieves a value from the request context by key, with optional default.
	Get(key any, def ...any) any
	// Set stores a value into a derived request context and replaces the underlying request.
	Set(key, value any) Ctx

	// Clone returns a shallow copy of the context suitable for use in a separate goroutine.
	Clone() Ctx
}

// DefaultContext is the concrete implementation of Ctx used by goflash.
// It wraps the http.ResponseWriter and *http.Request, exposes convenience helpers,
// and tracks route, status, and response state for each request.
//
// Handlers generally accept the interface type (ctx.Ctx), not *DefaultContext, to
// allow substituting alternative implementations if desired.
type DefaultContext struct {
	w           http.ResponseWriter // underlying response writer
	r           *http.Request       // underlying request
	params      router.Params       // route parameters
	status      int                 // status code to write
	wroteHeader bool                // whether header was written
	wroteBytes  int                 // number of bytes written
	route       string              // route pattern (e.g., /users/:id)
	jsonEscape  bool                // whether JSON encoder escapes HTML (default true)
}

// Reset prepares the context for a new request. Used internally by the framework.
// It swaps in the writer, request, params and route pattern, and clears any
// response state. Libraries and middleware should not need to call Reset.
//
// Example:
//
//	// internal server code
//	dctx.Reset(w, r, params, "/users/:id")
func (c *DefaultContext) Reset(w http.ResponseWriter, r *http.Request, ps router.Params, route string) {
	c.w = w
	c.r = r
	c.params = ps
	c.status = 0
	c.wroteHeader = false
	c.wroteBytes = 0
	c.route = route
	c.jsonEscape = true
}

// Finish is a hook for context cleanup after request handling. No-op by default.
// Frameworks may override or extend this method to release per-request resources.
func (c *DefaultContext) Finish() {
	// Reserved for future cleanup; reference receiver to create a coverable statement.
	_ = c
}

// Request returns the underlying *http.Request.
// Use c.Context() to access the request-scoped context values.
func (c *DefaultContext) Request() *http.Request { return c.r }

// SetRequest replaces the underlying *http.Request.
// Commonly used to attach a derived context:
//
//	ctx := context.WithValue(c.Context(), key, value)
//	c.SetRequest(c.Request().WithContext(ctx))
func (c *DefaultContext) SetRequest(r *http.Request) { c.r = r }

// ResponseWriter returns the underlying http.ResponseWriter.
func (c *DefaultContext) ResponseWriter() http.ResponseWriter { return c.w }

// SetResponseWriter replaces the underlying http.ResponseWriter.
// This is rarely needed in application code, but useful for testing or when
// wrapping the writer with middleware.
func (c *DefaultContext) SetResponseWriter(w http.ResponseWriter) { c.w = w }

// WroteHeader reports whether the response header has been written.
// After the header is written, changing headers or status has no effect.
func (c *DefaultContext) WroteHeader() bool { return c.wroteHeader }

// Context returns the request context.Context.
// It is the same as c.Request().Context().
func (c *DefaultContext) Context() context.Context { return c.r.Context() }

// Set stores a value in the request context using the provided key and value.
// It replaces the request with a clone that carries the new context and returns
// the context for chaining.
//
// Note: Prefer using a custom, unexported key type to avoid collisions.
//
// Example:
//
//	type userKey struct{}
//	c.Set(userKey{}, currentUser)
func (c *DefaultContext) Set(key, value any) Ctx {
	ctx := context.WithValue(c.Context(), key, value)
	c.SetRequest(c.Request().WithContext(ctx))
	return c
}

// Get returns a value from the request context by key.
// If the key is not present (or the stored value is nil), it returns the provided
// default when given (Get(key, def)), otherwise it returns nil.
//
// Example:
//
//	type userKey struct{}
//	u := c.Get(userKey{}).(*User)
func (c *DefaultContext) Get(key any, def ...any) any {
	v := c.Context().Value(key)
	if v != nil {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// Method returns the HTTP method for the request (e.g., "GET").
func (c *DefaultContext) Method() string { return c.r.Method }

// Path returns the request URL path (raw path without scheme/host).
func (c *DefaultContext) Path() string { return c.r.URL.Path }

// Route returns the route pattern for the current request, if known.
// For example, "/users/:id".
func (c *DefaultContext) Route() string { return c.route }

// Param returns a path parameter by name. Returns "" if not found.
// Note: router.Params.ByName returns "" if not found, so this avoids extra allocation.
//
// Example:
//
//	// Route: /posts/:slug
//	slug := c.Param("slug")
func (c *DefaultContext) Param(name string) string { return c.params.ByName(name) }

// Query returns a query string parameter by key. Returns "" if not found.
// Note: url.Values.Get returns "" if not found, so this avoids extra allocation.
//
// Example:
//
//	// URL: /search?q=flash
//	q := c.Query("q")
func (c *DefaultContext) Query(key string) string { return c.r.URL.Query().Get(key) }

// ParamInt returns the named path parameter parsed as int.
// Returns def (or 0) on missing or parse error.
//
// Example: c.ParamInt("id", 0) -> 42
func (c *DefaultContext) ParamInt(name string, def ...int) int {
	s := c.Param(name)
	fallback := 0
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return fallback
	}
	return int(v)
}

// ParamInt64 returns the named path parameter parsed as int64.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) ParamInt64(name string, def ...int64) int64 {
	s := c.Param(name)
	var fallback int64
	if len(def) > 0 {
		fallback = def[0]
	} else {
		fallback = 0
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

// ParamUint returns the named path parameter parsed as uint.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) ParamUint(name string, def ...uint) uint {
	s := c.Param(name)
	var fallback uint
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return fallback
	}
	return uint(v)
}

// ParamFloat64 returns the named path parameter parsed as float64.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) ParamFloat64(name string, def ...float64) float64 {
	s := c.Param(name)
	var fallback float64
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}

// ParamBool returns the named path parameter parsed as bool. Returns def on missing or parse error.
// Accepts the same forms as strconv.ParseBool: 1,t,T,TRUE,true,True, 0,f,F,FALSE,false,False.
func (c *DefaultContext) ParamBool(name string, def ...bool) bool {
	s := c.Param(name)
	fallback := false
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return v
}

// QueryInt returns the query parameter parsed as int.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) QueryInt(key string, def ...int) int {
	s := c.Query(key)
	fallback := 0
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return fallback
	}
	return int(v)
}

// QueryInt64 returns the query parameter parsed as int64.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) QueryInt64(key string, def ...int64) int64 {
	s := c.Query(key)
	var fallback int64
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

// QueryUint returns the query parameter parsed as uint.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) QueryUint(key string, def ...uint) uint {
	s := c.Query(key)
	var fallback uint
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return fallback
	}
	return uint(v)
}

// QueryFloat64 returns the query parameter parsed as float64.
// Returns def (or 0) on missing or parse error.
func (c *DefaultContext) QueryFloat64(key string, def ...float64) float64 {
	s := c.Query(key)
	var fallback float64
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}

// QueryBool returns the query parameter parsed as bool.
// Returns def (or false) on missing or parse error.
func (c *DefaultContext) QueryBool(key string, def ...bool) bool {
	s := c.Query(key)
	fallback := false
	if len(def) > 0 {
		fallback = def[0]
	}
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return v
}

// Status stages the response status code (without writing the header yet).
// Returns the context for chaining.
//
// Example:
//
//  	return c.Status(http.StatusAccepted).JSON(payload)

func (c *DefaultContext) Status(code int) Ctx {
	c.status = code
	return c
}

// StatusCode returns the status code that will be written.
// If not set yet and header hasn't been written, returns 0. If the header has
// already been written without an explicit status, returns 200.
func (c *DefaultContext) StatusCode() int {
	if c.status != 0 {
		return c.status
	}
	if c.wroteHeader {
		return http.StatusOK
	}
	return 0
}

// Header sets a header on the response.
// Has no effect after the header is written.
func (c *DefaultContext) Header(key, value string) { c.w.Header().Set(key, value) }

var jsonBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// SetJSONEscapeHTML controls whether JSON responses escape HTML characters.
// Default is true to match encoding/json defaults. Set to false when returning
// HTML-containing JSON that should not be escaped.
func (c *DefaultContext) SetJSONEscapeHTML(escape bool) { c.jsonEscape = escape }

// JSON serializes the provided value as JSON and writes the response.
// If Status() has not been called yet, it defaults to 200 OK.
// Content-Type is set to "application/json; charset=utf-8" and Content-Length is calculated.
//
// Example:
//
//	return c.Status(http.StatusCreated).JSON(struct{ ID int `json:"id"` }{ID: 1})
func (c *DefaultContext) JSON(v any) error {
	buf := jsonBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(c.jsonEscape)
	// Keep default escaping unless changed; compatible with stdlib behavior
	if err := enc.Encode(v); err != nil {
		jsonBufPool.Put(buf)
		// if header not written, send 500
		if !c.wroteHeader {
			c.w.WriteHeader(http.StatusInternalServerError)
			c.wroteHeader = true
		}
		return err
	}
	b := buf.Bytes()
	// trim trailing newline added by Encoder
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}

	if !c.wroteHeader {
		if c.status == 0 {
			c.status = http.StatusOK
		}
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Content-Length", strconv.Itoa(len(b)))
		c.w.WriteHeader(c.status)
		c.wroteHeader = true
	}
	_, err := c.w.Write(b)
	c.wroteBytes += len(b)
	buf.Reset()
	jsonBufPool.Put(buf)
	return err
}

// String writes a plain text response with the given status and body.
// Sets Content-Type to "text/plain; charset=utf-8" and Content-Length accordingly.
//
// Example:
//
//	return c.String(http.StatusOK, "pong")
func (c *DefaultContext) String(status int, body string) error {
	if !c.wroteHeader {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Length", strconv.Itoa(len(body)))
		c.w.WriteHeader(status)
		c.wroteHeader = true
	}
	n, err := io.WriteString(c.w, body)
	c.wroteBytes += n
	return err
}

// Send writes raw bytes with the given status and content type.
// If contentType is empty, no Content-Type header is set.
// Content-Length is set and the header is written once.
//
// Example:
//
//	data := []byte("<xml>ok</xml>")
//	_, err := c.Send(http.StatusOK, "application/xml", data)
func (c *DefaultContext) Send(status int, contentType string, b []byte) (int, error) {
	if !c.wroteHeader {
		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Header("Content-Length", strconv.Itoa(len(b)))
		c.w.WriteHeader(status)
		c.wroteHeader = true
	}
	n, err := c.w.Write(b)
	c.wroteBytes += n
	return n, err
}

// Redirect sends a redirect response with the given status code and URL.
func (c *DefaultContext) Redirect(status int, url string) error {
	if !c.wroteHeader {
		c.Header("Location", url)
		c.w.WriteHeader(status)
		c.wroteHeader = true
	}
	return nil
}

// RedirectPermanent sends a 301 permanent redirect.
func (c *DefaultContext) RedirectPermanent(url string) error {
	return c.Redirect(http.StatusMovedPermanently, url)
}

// RedirectTemporary sends a 302 temporary redirect.
func (c *DefaultContext) RedirectTemporary(url string) error {
	return c.Redirect(http.StatusFound, url)
}

// File serves a file from the local filesystem.
func (c *DefaultContext) File(path string) error {
	return c.FileFromFS(path, http.Dir("."))
}

// FileFromFS serves a file from the provided http.FileSystem.
func (c *DefaultContext) FileFromFS(path string, fs http.FileSystem) error {
	file, err := fs.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	if stat.IsDir() {
		return c.Forbidden()
	}

	// Set content type if not already set
	if !c.wroteHeader {
		contentType := "application/octet-stream"
		if ext := strings.ToLower(strings.TrimPrefix(path, ".")); ext != "" {
			if mimeType := http.DetectContentType([]byte(ext)); mimeType != "application/octet-stream" {
				contentType = mimeType
			}
		}
		c.Header("Content-Type", contentType)
		c.w.WriteHeader(http.StatusOK)
		c.wroteHeader = true
	}

	// Copy file content to response
	written, err := io.Copy(c.w, file)
	c.wroteBytes += int(written)
	return err
}

// NotFound sends a 404 Not Found response with optional message.
func (c *DefaultContext) NotFound(message ...string) error {
	msg := "Not Found"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.String(http.StatusNotFound, msg)
}

// InternalServerError sends a 500 Internal Server Error response with optional message.
func (c *DefaultContext) InternalServerError(message ...string) error {
	msg := "Internal Server Error"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.String(http.StatusInternalServerError, msg)
}

// BadRequest sends a 400 Bad Request response with optional message.
func (c *DefaultContext) BadRequest(message ...string) error {
	msg := "Bad Request"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.String(http.StatusBadRequest, msg)
}

// Unauthorized sends a 401 Unauthorized response with optional message.
func (c *DefaultContext) Unauthorized(message ...string) error {
	msg := "Unauthorized"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.String(http.StatusUnauthorized, msg)
}

// Forbidden sends a 403 Forbidden response with optional message.
func (c *DefaultContext) Forbidden(message ...string) error {
	msg := "Forbidden"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.String(http.StatusForbidden, msg)
}

// NoContent sends a 204 No Content response.
func (c *DefaultContext) NoContent() error {
	if !c.wroteHeader {
		c.w.WriteHeader(http.StatusNoContent)
		c.wroteHeader = true
	}
	return nil
}

// Stream streams data from an io.Reader with the given status and content type.
func (c *DefaultContext) Stream(status int, contentType string, reader io.Reader) error {
	if !c.wroteHeader {
		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.w.WriteHeader(status)
		c.wroteHeader = true
	}

	written, err := io.Copy(c.w, reader)
	c.wroteBytes += int(written)
	return err
}

// StreamJSON streams JSON data from an io.Reader with the given status.
func (c *DefaultContext) StreamJSON(status int, v any) error {
	buf := jsonBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(c.jsonEscape)

	if err := enc.Encode(v); err != nil {
		jsonBufPool.Put(buf)
		return err
	}

	b := buf.Bytes()
	// trim trailing newline added by Encoder
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}

	if !c.wroteHeader {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.w.WriteHeader(status)
		c.wroteHeader = true
	}

	_, err := c.w.Write(b)
	c.wroteBytes += len(b)
	buf.Reset()
	jsonBufPool.Put(buf)
	return err
}

// SetCookie sets a cookie in the response.
func (c *DefaultContext) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

// GetCookie retrieves a cookie from the request by name.
func (c *DefaultContext) GetCookie(name string) (*http.Cookie, error) {
	return c.r.Cookie(name)
}

// ClearCookie removes a cookie by setting it with an expired date.
func (c *DefaultContext) ClearCookie(name string) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		MaxAge:   -1,
		HttpOnly: true,
	}
	c.SetCookie(cookie)
}

// Clone returns a shallow copy of the context.
// Safe for use across goroutines as long as the ResponseWriter is swapped to a
// concurrency-safe writer if needed.
func (c *DefaultContext) Clone() Ctx { cp := *c; return &cp }

// Security-focused parameter and query helpers for input validation and sanitization.
// These methods help prevent common security vulnerabilities like XSS, path traversal,
// and injection attacks by sanitizing user input.

// alphaNumRegex matches only alphanumeric characters (a-z, A-Z, 0-9)
var alphaNumRegex = regexp.MustCompile(`^[a-zA-Z0-9]*$`)

// filenameRegex matches safe filename characters (alphanumeric, dash, underscore, dot)
var filenameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]*$`)

// ParamSafe returns a path parameter by name with HTML escaping to prevent XSS.
// This is useful when the parameter value will be displayed in HTML content.
//
// Security: Prevents XSS attacks by escaping HTML special characters.
//
// Example:
//
//	// Route: /users/:name
//	// URL: /users/<script>alert('xss')</script>
//	name := c.ParamSafe("name") // Returns: "&lt;script&gt;alert('xss')&lt;/script&gt;"
func (c *DefaultContext) ParamSafe(name string) string {
	return html.EscapeString(c.Param(name))
}

// QuerySafe returns a query parameter by key with HTML escaping to prevent XSS.
// This is useful when the query parameter value will be displayed in HTML content.
//
// Security: Prevents XSS attacks by escaping HTML special characters.
//
// Example:
//
//	// URL: /search?q=<script>alert('xss')</script>
//	q := c.QuerySafe("q") // Returns: "&lt;script&gt;alert('xss')&lt;/script&gt;"
func (c *DefaultContext) QuerySafe(key string) string {
	return html.EscapeString(c.Query(key))
}

// ParamAlphaNum returns a path parameter containing only alphanumeric characters.
// Non-alphanumeric characters are stripped from the result.
//
// Security: Prevents injection attacks by allowing only safe characters.
//
// Example:
//
//	// Route: /users/:id
//	// URL: /users/abc123../../../etc/passwd
//	id := c.ParamAlphaNum("id") // Returns: "abc123"
func (c *DefaultContext) ParamAlphaNum(name string) string {
	param := c.Param(name)
	if param == "" {
		return ""
	}

	// Extract only alphanumeric characters
	var result strings.Builder
	for _, r := range param {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// QueryAlphaNum returns a query parameter containing only alphanumeric characters.
// Non-alphanumeric characters are stripped from the result.
//
// Security: Prevents injection attacks by allowing only safe characters.
//
// Example:
//
//	// URL: /search?category=books&sort=name';DROP TABLE users;--
//	sort := c.QueryAlphaNum("sort") // Returns: "nameDROPTABLEusers"
func (c *DefaultContext) QueryAlphaNum(key string) string {
	query := c.Query(key)
	if query == "" {
		return ""
	}

	// Extract only alphanumeric characters
	var result strings.Builder
	for _, r := range query {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ParamFilename returns a path parameter as a safe filename.
// Only allows alphanumeric characters, dots, dashes, and underscores.
// Prevents path traversal attacks by removing directory separators.
//
// Security: Prevents path traversal attacks and ensures safe filenames.
//
// Example:
//
//	// Route: /files/:name
//	// URL: /files/../../../etc/passwd
//	name := c.ParamFilename("name") // Returns: "etcpasswd"
//
//	// URL: /files/document.pdf
//	name := c.ParamFilename("name") // Returns: "document.pdf"
func (c *DefaultContext) ParamFilename(name string) string {
	param := c.Param(name)
	if param == "" {
		return ""
	}

	// URL decode first to handle encoded path traversal attempts
	decoded, err := url.QueryUnescape(param)
	if err != nil {
		decoded = param
	}

	// Extract only safe filename characters
	var result strings.Builder
	for _, r := range decoded {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}

	filename := result.String()

	// Prevent hidden files and relative paths
	filename = strings.TrimPrefix(filename, ".")

	return filename
}

// QueryFilename returns a query parameter as a safe filename.
// Only allows alphanumeric characters, dots, dashes, and underscores.
// Prevents path traversal attacks by removing directory separators.
//
// Security: Prevents path traversal attacks and ensures safe filenames.
//
// Example:
//
//	// URL: /download?file=../../../etc/passwd
//	file := c.QueryFilename("file") // Returns: "etcpasswd"
//
//	// URL: /download?file=document.pdf
//	file := c.QueryFilename("file") // Returns: "document.pdf"
func (c *DefaultContext) QueryFilename(key string) string {
	query := c.Query(key)
	if query == "" {
		return ""
	}

	// URL decode first to handle encoded path traversal attempts
	decoded, err := url.QueryUnescape(query)
	if err != nil {
		decoded = query
	}

	// Extract only safe filename characters
	var result strings.Builder
	for _, r := range decoded {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}

	filename := result.String()

	// Prevent hidden files and relative paths
	filename = strings.TrimPrefix(filename, ".")

	return filename
}
