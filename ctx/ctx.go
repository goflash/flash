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

// Ctx is the request context for goflash handlers and middleware.
// It wraps the http.ResponseWriter and *http.Request, exposes convenience helpers,
// and tracks route, status, and response state for each request.
type Ctx struct {
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
func (c *Ctx) Reset(w http.ResponseWriter, r *http.Request, ps httprouter.Params, route string) {
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
func (c *Ctx) Finish() {
	// Reserved for future cleanup; reference receiver to create a coverable statement.
	_ = c
}

// Request returns the underlying *http.Request.
func (c *Ctx) Request() *http.Request { return c.r }

// SetRequest replaces the underlying *http.Request.
func (c *Ctx) SetRequest(r *http.Request) { c.r = r }

// ResponseWriter returns the underlying http.ResponseWriter.
func (c *Ctx) ResponseWriter() http.ResponseWriter { return c.w }

// SetResponseWriter replaces the underlying http.ResponseWriter.
func (c *Ctx) SetResponseWriter(w http.ResponseWriter) { c.w = w }

// WroteHeader reports whether the response header has been written.
func (c *Ctx) WroteHeader() bool { return c.wroteHeader }

// Context returns the request context.Context.
func (c *Ctx) Context() context.Context { return c.r.Context() }

// Method returns the HTTP method for the request.
func (c *Ctx) Method() string { return c.r.Method }

// Path returns the request URL path.
func (c *Ctx) Path() string { return c.r.URL.Path }

// Route returns the route pattern for the current request.
func (c *Ctx) Route() string { return c.route }

// Param returns a path parameter by name. Returns "" if not found.
// Note: httprouter.Params.ByName returns "" if not found, so this avoids extra allocation.
func (c *Ctx) Param(name string) string { return c.params.ByName(name) }

// Query returns a query string parameter by key. Returns "" if not found.
// Note: url.Values.Get returns "" if not found, so this avoids extra allocation.
func (c *Ctx) Query(key string) string { return c.r.URL.Query().Get(key) }

// Status sets the response status code (without writing the header yet).
// Returns the context for chaining.
func (c *Ctx) Status(code int) *Ctx {
	c.status = code
	return c
}

// StatusCode returns the status code that will be written (or 200 if not set yet).
func (c *Ctx) StatusCode() int {
	if c.status != 0 {
		return c.status
	}
	if c.wroteHeader {
		return http.StatusOK
	}
	return 0
}

// Header sets a header on the response.
func (c *Ctx) Header(key, value string) { c.w.Header().Set(key, value) }

var jsonBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// SetJSONEscapeHTML controls whether JSON responses escape HTML characters (default true).
func (c *Ctx) SetJSONEscapeHTML(escape bool) { c.jsonEscape = escape }

// JSON writes the provided value as JSON with status code (defaults to 200 if not set).
func (c *Ctx) JSON(v any) error {
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
func (c *Ctx) String(status int, body string) error {
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
func (c *Ctx) Send(status int, contentType string, b []byte) (int, error) {
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
func (c *Ctx) BindJSON(v any, opts ...BindJSONOptions) error {
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
