package ctx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRequest(method, target string, body io.Reader) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, target, body)
	rec := httptest.NewRecorder()
	return req, rec
}

func TestStringWritesStatusHeadersAndBody(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	assert.False(t, c.WroteHeader())
	require.NoError(t, c.String(http.StatusCreated, "hello"))
	assert.True(t, c.WroteHeader())
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "5", rec.Header().Get("Content-Length"))
	assert.Equal(t, "hello", rec.Body.String())
}

func TestJSONWritesAndDefaultsAndEscape(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	type payload struct {
		Msg string `json:"msg"`
	}
	p := payload{Msg: "<ok>"}
	require.NoError(t, c.JSON(p))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	// Default escape is true, so '<' should be escaped and value is a JSON string
	assert.Equal(t, "{\"msg\":\"\\u003cok\\u003e\"}", rec.Body.String())
}

func TestJSONEscapeDisabled(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type payload struct {
		Msg string `json:"msg"`
	}
	p := payload{Msg: "<ok>"}
	c.SetJSONEscapeHTML(false)
	require.NoError(t, c.JSON(p))
	assert.Equal(t, "{\"msg\":\"<ok>\"}", rec.Body.String())
}

func TestSendWritesBytesAndHeaders(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	n, err := c.Send(418, "application/octet-stream", []byte{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 418, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "3", rec.Header().Get("Content-Length"))
}

func TestSendEmptyContentType(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	_, _ = c.Send(200, "", []byte("x"))
	if rec.Header().Get("Content-Length") != "1" {
		t.Fatalf("missing CL")
	}
}

func TestHeaderAndStatusCode(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	c.Header("X-Test", "1")
	c.Status(http.StatusAccepted)
	require.NoError(t, c.String(http.StatusAccepted, "ok"))
	assert.Equal(t, "1", rec.Header().Get("X-Test"))
	assert.Equal(t, http.StatusAccepted, c.StatusCode())
}

func TestParamQueryMethodPathRoute(t *testing.T) {
	u := &url.URL{Scheme: "http", Host: "example.com", Path: "/users/123", RawQuery: "q=go"}
	req := &http.Request{Method: http.MethodGet, URL: u}
	rec := httptest.NewRecorder()
	ps := httprouter.Params{httprouter.Param{Key: "id", Value: "123"}}
	var c DefaultContext
	c.Reset(rec, req, ps, "/users/:id")
	assert.Equal(t, "GET", c.Method())
	assert.Equal(t, "/users/123", c.Path())
	assert.Equal(t, "/users/:id", c.Route())
	assert.Equal(t, "123", c.Param("id"))
	assert.Equal(t, "go", c.Query("q"))
}

func TestTypedParamHelpers(t *testing.T) {
	u := &url.URL{Scheme: "http", Host: "example.com", Path: "/u/42/3.14/true"}
	req := &http.Request{Method: http.MethodGet, URL: u}
	rec := httptest.NewRecorder()
	ps := httprouter.Params{
		{Key: "id", Value: "42"},
		{Key: "pi", Value: "3.14"},
		{Key: "ok", Value: "true"},
	}
	var c DefaultContext
	c.Reset(rec, req, ps, "/u/:id/:pi/:ok")

	assert.Equal(t, 42, c.ParamInt("id", -1))
	assert.Equal(t, int64(42), c.ParamInt64("id", -2))
	assert.Equal(t, uint(42), c.ParamUint("id", 9))
	assert.InDelta(t, 3.14, c.ParamFloat64("pi", 0), 0.0001)
	assert.Equal(t, true, c.ParamBool("ok", false))

	// Missing or invalid -> default
	assert.Equal(t, -1, c.ParamInt("missing", -1))
	// No default provided -> zero value
	assert.Equal(t, 0, c.ParamInt("missing"))
	ps = append(ps, httprouter.Param{Key: "bad", Value: "xx"})
	c.params = ps
	assert.Equal(t, 7, c.ParamInt("bad", 7))
}

func TestTypedQueryHelpers(t *testing.T) {
	q := url.Values{}
	q.Set("n", "10")
	q.Set("big", "9007199254740991") // < 2^53
	q.Set("u", "11")
	q.Set("f", "2.5")
	q.Set("b", "true")
	q.Set("bad", "nope")
	u := &url.URL{Scheme: "http", Host: "example.com", Path: "/", RawQuery: q.Encode()}
	req := &http.Request{Method: http.MethodGet, URL: u}
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	assert.Equal(t, 10, c.QueryInt("n", -1))
	assert.Equal(t, int64(9007199254740991), c.QueryInt64("big", -2))
	assert.Equal(t, uint(11), c.QueryUint("u", 0))
	assert.InDelta(t, 2.5, c.QueryFloat64("f", 0), 0.0001)
	assert.Equal(t, true, c.QueryBool("b", false))

	// Missing/invalid
	assert.Equal(t, -9, c.QueryInt("missing", -9))
	// No default provided -> zero value
	assert.Equal(t, 0, c.QueryInt("missing"))
	assert.Equal(t, 3, c.QueryInt("bad", 3))
	assert.Equal(t, false, c.QueryBool("bad", false))
}

func TestJSONErrorSets500(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type bad struct{ F func() }
	err := c.JSON(bad{})
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStatusCodeDefaultWhenHeaderNotWritten(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	c.Status(http.StatusAccepted)
	// before write, StatusCode should be 202
	assert.Equal(t, http.StatusAccepted, c.StatusCode())
}

func TestJSONSetsContentLengthAndTrimsNewline(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type P struct {
		A int `json:"a"`
	}
	_ = c.JSON(P{A: 1})
	if cl := rec.Header().Get("Content-Length"); cl == "" {
		t.Fatalf("missing content-length")
	}
	if bytes.HasSuffix(rec.Body.Bytes(), []byte{'\n'}) {
		t.Fatalf("unexpected trailing newline")
	}
}

func TestCtxAccessorsCoverage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/p?q=1", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/p")
	if c.Request() == nil || c.ResponseWriter() == nil || c.Context() == nil {
		t.Fatalf("accessors nil")
	}
	// SetRequest/ResponseWriter
	r2 := req.Clone(req.Context())
	c.SetRequest(r2)
	c.SetResponseWriter(rec)
	_ = c.WroteHeader()
}

func TestFinishAndAccessorsCoverage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	_ = c.ResponseWriter()
	_ = c.Request()
	c.Finish()
}

func TestStatusCodeBranchesV2(t *testing.T) {
	var c DefaultContext
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c.Reset(rec, req, nil, "/")
	// initially 0
	if c.StatusCode() != 0 {
		t.Fatalf("want 0")
	}
	// after write without explicit status -> 200
	_ = c.String(http.StatusOK, "ok")
	if c.StatusCode() != http.StatusOK {
		t.Fatalf("want 200")
	}
	c.Finish()
}

// New tests for Set/Get helpers
func TestCtx_SetGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// missing with no default -> nil
	if got := c.Get("k"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	// missing with default -> default
	if got := c.Get("k", "def"); got != "def" {
		t.Fatalf("expected default, got %v", got)
	}
	// set -> read
	c.Set("k", "v")
	if got := c.Get("k", "def"); got != "v" {
		t.Fatalf("expected 'v', got %v", got)
	}
}

func TestCtx_Clone_ShallowCopy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/a/123?x=1", nil)
	rec := httptest.NewRecorder()
	ps := httprouter.Params{{Key: "id", Value: "123"}}
	var c DefaultContext
	c.Reset(rec, req, ps, "/a/:id")
	c.Header("X-Test", "1")
	c.Status(202)

	clone := c.Clone()
	// Assert basic properties copied
	assert.Equal(t, "/a/123", clone.Path())
	assert.Equal(t, "/a/:id", clone.Route())
	assert.Equal(t, "123", clone.Param("id"))
	assert.Equal(t, "1", clone.Query("x"))
	// Mutate clone status and ensure original remains unchanged
	_ = clone.Status(201)
	// original still has 202 pending
	assert.Equal(t, 202, c.StatusCode())
}

func TestJSONWithPresetStatus(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	c.Status(http.StatusCreated)
	err := c.JSON(map[string]any{"ok": true})
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)
	// content-length should be set
	if rec.Header().Get("Content-Length") == "" {
		t.Fatalf("missing content-length")
	}
}

func TestTypedHelpers_DefaultAndNoDefaultFallbacks(t *testing.T) {
	// Prepare request with invalid query values
	q := url.Values{}
	q.Set("qi", "bad")
	q.Set("qi64", "bad")
	q.Set("qu", "-1") // invalid for uint
	q.Set("qf", "bad")
	q.Set("qb", "maybe")
	u := &url.URL{Scheme: "http", Host: "example.com", Path: "/p/xx/yy/-1/x/maybe", RawQuery: q.Encode()}
	req := &http.Request{Method: http.MethodGet, URL: u}
	rec := httptest.NewRecorder()
	ps := httprouter.Params{
		{Key: "pi", Value: "xx"},    // invalid int
		{Key: "pi64", Value: "yy"},  // invalid int64
		{Key: "pu", Value: "-1"},    // invalid uint
		{Key: "pf", Value: "x"},     // invalid float
		{Key: "pb", Value: "maybe"}, // invalid bool
	}
	var c DefaultContext
	c.Reset(rec, req, ps, "/p/:pi/:pi64/:pu/:pf/:pb")

	// Path params with defaults
	assert.Equal(t, 5, c.ParamInt("pi", 5))
	assert.Equal(t, int64(6), c.ParamInt64("pi64", 6))
	assert.Equal(t, uint(7), c.ParamUint("pu", 7))
	assert.Equal(t, 1.25, c.ParamFloat64("pf", 1.25))
	assert.Equal(t, true, c.ParamBool("pb", true))

	// Path params without defaults -> zero values
	assert.Equal(t, 0, c.ParamInt("pi"))
	assert.Equal(t, int64(0), c.ParamInt64("pi64"))
	assert.Equal(t, uint(0), c.ParamUint("pu"))
	assert.Equal(t, 0.0, c.ParamFloat64("pf"))
	assert.Equal(t, false, c.ParamBool("pb"))

	// Missing keys -> fallback and zero
	assert.Equal(t, 9, c.ParamInt("missing", 9))
	assert.Equal(t, 0, c.ParamInt("missing"))
	assert.Equal(t, uint(11), c.ParamUint("missingU", 11))
	assert.Equal(t, uint(0), c.ParamUint("missingU"))
	assert.Equal(t, 2.75, c.ParamFloat64("missingF", 2.75))
	assert.Equal(t, 0.0, c.ParamFloat64("missingF"))
	assert.Equal(t, true, c.ParamBool("missingB", true))
	assert.Equal(t, false, c.ParamBool("missingB"))

	// Query params invalid -> fallback and zero
	assert.Equal(t, 3, c.QueryInt("qi", 3))
	assert.Equal(t, 0, c.QueryInt("qi"))
	assert.Equal(t, int64(4), c.QueryInt64("qi64", 4))
	assert.Equal(t, int64(0), c.QueryInt64("qi64"))
	assert.Equal(t, uint(5), c.QueryUint("qu", 5))
	assert.Equal(t, uint(0), c.QueryUint("qu"))
	assert.Equal(t, 6.5, c.QueryFloat64("qf", 6.5))
	assert.Equal(t, 0.0, c.QueryFloat64("qf"))
	assert.Equal(t, true, c.QueryBool("qb", true))
	assert.Equal(t, false, c.QueryBool("qb"))

	// Missing query keys -> fallback and zero
	assert.Equal(t, 8, c.QueryInt("missingQi", 8))
	assert.Equal(t, 0, c.QueryInt("missingQi"))
	assert.Equal(t, uint(9), c.QueryUint("missingQu", 9))
	assert.Equal(t, uint(0), c.QueryUint("missingQu"))
}

func TestParamInt64WithInvalidValue(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/user/invalid", nil)
	var c DefaultContext
	c.Reset(rec, req, httprouter.Params{{Key: "id", Value: "invalid"}}, "/user/:id")

	// Should return default value when parsing fails
	assert.Equal(t, int64(42), c.ParamInt64("id", 42))
	assert.Equal(t, int64(0), c.ParamInt64("id"))
}

func TestQueryInt64WithInvalidValue(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?id=invalid", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Should return default value when parsing fails
	assert.Equal(t, int64(42), c.QueryInt64("id", 42))
	assert.Equal(t, int64(0), c.QueryInt64("id"))
}

func TestQueryFloat64WithInvalidValue(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?price=invalid", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Should return default value when parsing fails
	assert.Equal(t, 99.99, c.QueryFloat64("price", 99.99))
	assert.Equal(t, 0.0, c.QueryFloat64("price"))
}

func TestQueryBoolWithInvalidValue(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?active=invalid", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Should return default value when parsing fails
	assert.Equal(t, true, c.QueryBool("active", true))
	assert.Equal(t, false, c.QueryBool("active"))
}

func TestParamFilenameWithInvalidCharacters(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/file/invalid..file", nil)
	var c DefaultContext
	c.Reset(rec, req, httprouter.Params{{Key: "filename", Value: "invalid..file"}}, "/file/:filename")

	// Should return sanitized filename (dots are allowed)
	result := c.ParamFilename("filename")
	expected := "invalid..file" // dots are safe filename characters
	assert.Equal(t, expected, result)
}

func TestQueryFilenameWithInvalidCharacters(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?file=invalid..file", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Should return sanitized filename (dots are allowed)
	result := c.QueryFilename("file")
	expected := "invalid..file" // dots are safe filename characters
	assert.Equal(t, expected, result)
}

func TestParamInt64EdgeCases(t *testing.T) {
	// Test ParamInt64 with various edge cases
	req, rec := newRequest(http.MethodGet, "/user/9223372036854775807", nil)
	var c DefaultContext
	c.Reset(rec, req, httprouter.Params{{Key: "id", Value: "9223372036854775807"}}, "/user/:id")

	// Test max int64
	assert.Equal(t, int64(9223372036854775807), c.ParamInt64("id"))

	// Test missing param
	assert.Equal(t, int64(0), c.ParamInt64("missing"))

	// Test with default
	assert.Equal(t, int64(42), c.ParamInt64("missing", 42))
}

func TestQueryInt64EdgeCases(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?id=9223372036854775807", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test max int64
	assert.Equal(t, int64(9223372036854775807), c.QueryInt64("id"))

	// Test missing query
	assert.Equal(t, int64(0), c.QueryInt64("missing"))

	// Test with default
	assert.Equal(t, int64(42), c.QueryInt64("missing", 42))
}

func TestQueryFloat64EdgeCases(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?price=123.456", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test valid float
	assert.Equal(t, 123.456, c.QueryFloat64("price"))

	// Test missing query
	assert.Equal(t, 0.0, c.QueryFloat64("missing"))

	// Test with default
	assert.Equal(t, 99.99, c.QueryFloat64("missing", 99.99))
}

func TestQueryBoolEdgeCases(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/?active=1&inactive=false", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test various true values
	assert.Equal(t, true, c.QueryBool("active"))
	assert.Equal(t, false, c.QueryBool("inactive"))

	// Test missing query
	assert.Equal(t, false, c.QueryBool("missing"))

	// Test with default
	assert.Equal(t, true, c.QueryBool("missing", true))
}

func TestParamFilenameEdgeCases(t *testing.T) {
	// Test with path traversal attempts
	req, rec := newRequest(http.MethodGet, "/file/..%2F..%2Fetc%2Fpasswd", nil)
	var c DefaultContext
	c.Reset(rec, req, httprouter.Params{{Key: "filename", Value: "../../../etc/passwd"}}, "/file/:filename")

	// Should sanitize path traversal
	result := c.ParamFilename("filename")
	expected := ".....etcpasswd" // dots are preserved, slashes removed
	assert.Equal(t, expected, result)

	// Test empty filename
	req2, rec2 := newRequest(http.MethodGet, "/file/", nil)
	c.Reset(rec2, req2, httprouter.Params{{Key: "filename", Value: ""}}, "/file/:filename")
	assert.Equal(t, "", c.ParamFilename("filename"))
}

func TestQueryFilenameEdgeCases(t *testing.T) {
	// Test with path traversal attempts
	req, rec := newRequest(http.MethodGet, "/?file=../../../etc/passwd", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Should sanitize path traversal
	result := c.QueryFilename("file")
	expected := ".....etcpasswd" // dots are preserved, slashes removed
	assert.Equal(t, expected, result)

	// Test empty query
	req2, rec2 := newRequest(http.MethodGet, "/", nil)
	c.Reset(rec2, req2, nil, "/")
	assert.Equal(t, "", c.QueryFilename("file"))
}

func TestParamFilenameWithSpecialCharacters(t *testing.T) {
	// Test ParamFilename with various special characters to hit uncovered branches
	testCases := []struct {
		input    string
		expected string
	}{
		{"file.txt", "file.txt"},                           // Normal case
		{"../../../etc/passwd", ".....etcpasswd"},          // Path traversal (dots preserved, slashes removed)
		{"file\\with\\backslashes", "filewithbackslashes"}, // Backslashes
		{"file/with/slashes", "filewithslashes"},           // Forward slashes
		{"file\x00null", "filenull"},                       // Null bytes
		{"", ""},                                           // Empty string
		{"...", ".."},                                      // Just dots (leading dot trimmed)
		{"file..name", "file..name"},                       // Multiple dots (preserved)
	}

	for _, tc := range testCases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		var c DefaultContext
		c.Reset(rec, req, httprouter.Params{{Key: "filename", Value: tc.input}}, "/test")

		result := c.ParamFilename("filename")
		if result != tc.expected {
			t.Errorf("ParamFilename(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestQueryFilenameWithSpecialCharacters(t *testing.T) {
	// Test QueryFilename with various special characters to hit uncovered branches
	testCases := []struct {
		input    string
		expected string
	}{
		{"file.txt", "file.txt"},                           // Normal case
		{"../../../etc/passwd", ".....etcpasswd"},          // Path traversal (dots preserved, slashes removed)
		{"file\\with\\backslashes", "filewithbackslashes"}, // Backslashes
		{"file/with/slashes", "filewithslashes"},           // Forward slashes
		{"file\x00null", "filenull"},                       // Null bytes
		{"", ""},                                           // Empty string
		{"...", ".."},                                      // Just dots (leading dot trimmed)
		{"file..name", "file..name"},                       // Multiple dots (preserved)
	}

	for _, tc := range testCases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test?filename="+url.QueryEscape(tc.input), nil)

		var c DefaultContext
		c.Reset(rec, req, nil, "/test")

		result := c.QueryFilename("filename")
		if result != tc.expected {
			t.Errorf("QueryFilename(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}
