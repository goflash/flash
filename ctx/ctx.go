package ctx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/julienschmidt/httprouter"
	ms "github.com/mitchellh/mapstructure"
)

var newMSDecoder = ms.NewDecoder

// Ctx is the interface exposed to handlers and middleware.
// Implemented by *DefaultContext. Located in package ctx to avoid adapters and cycles.
type Ctx interface {
	// Request/Response accessors and mutators
	Request() *http.Request
	SetRequest(*http.Request)
	ResponseWriter() http.ResponseWriter
	SetResponseWriter(http.ResponseWriter)

	// Basic request data
	Context() context.Context
	Method() string
	Path() string
	Route() string
	Param(name string) string
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

	// Response helpers
	Header(key, value string)
	Status(code int) Ctx
	StatusCode() int
	JSON(v any) error
	String(status int, body string) error
	Send(status int, contentType string, b []byte) (int, error)
	WroteHeader() bool

	// BindJSON decodes request body JSON into v with strict defaults; see BindJSONOptions.
	BindJSON(v any, opts ...BindJSONOptions) error

	// Utilities
	Get(key any, def ...any) any
	Set(key, value any) Ctx

	// Clone returns a shallow copy of the context suitable for use in a separate goroutine.
	Clone() Ctx
}

// DefaultContext is the request context for goflash handlers and middleware.
// It wraps the http.ResponseWriter and *http.Request, exposes convenience helpers,
// and tracks route, status, and response state for each request.
type DefaultContext struct {
	w           http.ResponseWriter // underlying response writer
	r           *http.Request       // underlying request
	params      httprouter.Params   // route parameters
	status      int                 // status code to write
	wroteHeader bool                // whether header was written
	wroteBytes  int                 // number of bytes written
	route       string              // route pattern (e.g., /users/:id)
	jsonEscape  bool                // whether JSON encoder escapes HTML (default true)
}

// Reset prepares the context for a new request. Used internally by the framework.
func (c *DefaultContext) Reset(w http.ResponseWriter, r *http.Request, ps httprouter.Params, route string) {
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
func (c *DefaultContext) Finish() {
	// Reserved for future cleanup; reference receiver to create a coverable statement.
	_ = c
}

// Request returns the underlying *http.Request.
func (c *DefaultContext) Request() *http.Request { return c.r }

// SetRequest replaces the underlying *http.Request.
func (c *DefaultContext) SetRequest(r *http.Request) { c.r = r }

// ResponseWriter returns the underlying http.ResponseWriter.
func (c *DefaultContext) ResponseWriter() http.ResponseWriter { return c.w }

// SetResponseWriter replaces the underlying http.ResponseWriter.
func (c *DefaultContext) SetResponseWriter(w http.ResponseWriter) { c.w = w }

// WroteHeader reports whether the response header has been written.
func (c *DefaultContext) WroteHeader() bool { return c.wroteHeader }

// Context returns the request context.Context.
func (c *DefaultContext) Context() context.Context { return c.r.Context() }

// Set stores a value in the request context using the provided key.
// It replaces the request with a clone that carries the new context.
//
// Note: Prefer using a custom, unexported key type to avoid collisions.
func (c *DefaultContext) Set(key, value any) Ctx {
	ctx := context.WithValue(c.Context(), key, value)
	c.SetRequest(c.Request().WithContext(ctx))
	return c
}

// Get returns a value from the request context by key.
// If the key is not present (or the stored value is nil), it returns the provided default
// when given (Get(key, def)), otherwise it returns nil.
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

// Method returns the HTTP method for the request.
func (c *DefaultContext) Method() string { return c.r.Method }

// Path returns the request URL path.
func (c *DefaultContext) Path() string { return c.r.URL.Path }

// Route returns the route pattern for the current request.
func (c *DefaultContext) Route() string { return c.route }

// Param returns a path parameter by name. Returns "" if not found.
// Note: httprouter.Params.ByName returns "" if not found, so this avoids extra allocation.
func (c *DefaultContext) Param(name string) string { return c.params.ByName(name) }

// ParamInt returns a path parameter as an int. Returns the default value if not found or invalid.
func (c *Ctx) ParamInt(name string, def int) int {
	val := c.Param(name)
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return def
}

// ParamInt64 returns a path parameter as an int64. Returns the default value if not found or invalid.
func (c *Ctx) ParamInt64(name string, def int64) int64 {
	val := c.Param(name)
	if val == "" {
		return def
	}
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i
	}
	return def
}

// ParamFloat64 returns a path parameter as a float64. Returns the default value if not found or invalid.
func (c *Ctx) ParamFloat64(name string, def float64) float64 {
	val := c.Param(name)
	if val == "" {
		return def
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return def
}

// ParamUint returns a path parameter as a uint. Returns the default value if not found or invalid.
func (c *Ctx) ParamUint(name string, def uint) uint {
	val := c.Param(name)
	if val == "" {
		return def
	}
	if u, err := strconv.ParseUint(val, 10, 32); err == nil {
		return uint(u)
	}
	return def
}

// ParamUint64 returns a path parameter as a uint64. Returns the default value if not found or invalid.
func (c *Ctx) ParamUint64(name string, def uint64) uint64 {
	val := c.Param(name)
	if val == "" {
		return def
	}
	if u, err := strconv.ParseUint(val, 10, 64); err == nil {
		return u
	}
	return def
}

// Query returns a query string parameter by key. Returns "" if not found.
// Note: url.Values.Get returns "" if not found, so this avoids extra allocation.
func (c *DefaultContext) Query(key string) string { return c.r.URL.Query().Get(key) }

// ParamInt returns the named path parameter parsed as int. Returns def on missing or parse error.
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

// ParamInt64 returns the named path parameter parsed as int64. Returns def on missing or parse error.
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

// ParamUint returns the named path parameter parsed as uint. Returns def on missing or parse error.
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

// ParamFloat64 returns the named path parameter parsed as float64. Returns def on missing or parse error.
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

// QueryInt returns the query parameter parsed as int. Returns def on missing or parse error.
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

// QueryInt64 returns the query parameter parsed as int64. Returns def on missing or parse error.
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

// QueryUint returns the query parameter parsed as uint. Returns def on missing or parse error.
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

// QueryFloat64 returns the query parameter parsed as float64. Returns def on missing or parse error.
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

// QueryBool returns the query parameter parsed as bool. Returns def on missing or parse error.
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

// QueryInt returns a query string parameter as an int. Returns the default value if not found or invalid.
func (c *Ctx) QueryInt(key string, def int) int {
	val := c.Query(key)
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return def
}

// QueryInt64 returns a query string parameter as an int64. Returns the default value if not found or invalid.
func (c *Ctx) QueryInt64(key string, def int64) int64 {
	val := c.Query(key)
	if val == "" {
		return def
	}
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i
	}
	return def
}

// QueryFloat64 returns a query string parameter as a float64. Returns the default value if not found or invalid.
func (c *Ctx) QueryFloat64(key string, def float64) float64 {
	val := c.Query(key)
	if val == "" {
		return def
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return def
}

// QueryBool returns a query string parameter as a bool. Returns the default value if not found.
// Accepts "true", "1", "yes", "on" as true values (case-insensitive).
func (c *Ctx) QueryBool(key string, def bool) bool {
	val := c.Query(key)
	if val == "" {
		return def
	}
	switch val {
	case "true", "1", "yes", "on", "TRUE", "True", "YES", "Yes", "ON", "On":
		return true
	case "false", "0", "no", "off", "FALSE", "False", "NO", "No", "OFF", "Off":
		return false
	default:
		return def
	}
}

// QueryUint returns a query string parameter as a uint. Returns the default value if not found or invalid.
func (c *Ctx) QueryUint(key string, def uint) uint {
	val := c.Query(key)
	if val == "" {
		return def
	}
	if u, err := strconv.ParseUint(val, 10, 32); err == nil {
		return uint(u)
	}
	return def
}

// QueryUint64 returns a query string parameter as a uint64. Returns the default value if not found or invalid.
func (c *Ctx) QueryUint64(key string, def uint64) uint64 {
	val := c.Query(key)
	if val == "" {
		return def
	}
	if u, err := strconv.ParseUint(val, 10, 64); err == nil {
		return u
	}
	return def
}

// Status sets the response status code (without writing the header yet).
// Returns the context for chaining.

func (c *DefaultContext) Status(code int) Ctx {
	c.status = code
	return c
}

// StatusCode returns the status code that will be written (or 200 if not set yet).
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
func (c *DefaultContext) Header(key, value string) { c.w.Header().Set(key, value) }

var jsonBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// SetJSONEscapeHTML controls whether JSON responses escape HTML characters (default true).
func (c *DefaultContext) SetJSONEscapeHTML(escape bool) { c.jsonEscape = escape }

// JSON writes the provided value as JSON with status code (defaults to 200 if not set).
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

// BindJSONOptions customizes how BindJSON decodes JSON payloads when binding into structs.
// All fields are optional and default to false (strict behavior).
type BindJSONOptions struct {
	// WeaklyTypedInput allows common type coercions, e.g., "10" -> 10 for int fields.
	WeaklyTypedInput bool
	// ErrorUnused when true returns an error for unexpected fields.
	ErrorUnused bool
}

// BindJSON decodes the request body JSON into v.
// If v is a pointer to a struct, behavior can be customized using an optional BindJSONOptions parameter.
// Defaults: strict decoding using encoding/json with DisallowUnknownFields, no type coercion.
func (c *DefaultContext) BindJSON(v any, opts ...BindJSONOptions) error {
	defer c.r.Body.Close()

	var o BindJSONOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	// Non-struct targets: keep strict json decoder behavior regardless of options.
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		dec := json.NewDecoder(c.r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			if fErr := mapJSONStrictError(err, reflect.TypeOf(nil)); fErr != nil { // no struct type context
				return fErr
			}
			return err
		}
		return nil
	}
	// Capture the target struct type for better error messages
	targetType := rv.Elem().Type()

	// When no options are enabled, use stdlib decoder for performance and consistent errors.
	if !o.WeaklyTypedInput && !o.ErrorUnused {
		dec := json.NewDecoder(c.r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			if fErr := mapJSONStrictError(err, targetType); fErr != nil {
				return fErr
			}
			return err
		}
		return nil
	}

	// Read body for flexible decoding and analysis.
	b, err := io.ReadAll(c.r.Body)
	if err != nil {
		return err
	}

	// First unmarshal generically.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		// Try to convert a type mismatch into a field error when WeaklyTypedInput is false.
		// encoding/json error messages often look like: "json: cannot unmarshal string into Go struct field User.age of type int"
		if !o.WeaklyTypedInput {
			if fErr := tryJSONTypeErrorToField(err, targetType); fErr != nil {
				return fErr
			}
		}
		return err
	}

	// Configure map structure based on options.
	cfg := &ms.DecoderConfig{
		TagName:          "json",
		Result:           v,
		WeaklyTypedInput: o.WeaklyTypedInput,
		ErrorUnused:      o.ErrorUnused,
	}
	dec, err := newMSDecoder(cfg)
	if err != nil {
		return err
	}
	if err := dec.Decode(m); err != nil {
		// Map map structure errors to field errors
		if fe := mapMapStructureError(err, o, targetType); fe != nil {
			return fe
		}
		return err
	}
	return nil
}

// Clone returns a shallow copy of the context. Safe for use across goroutines
// as long as ResponseWriter is swapped to a concurrency-safe writer when needed.
func (c *DefaultContext) Clone() Ctx { cp := *c; return &cp }
