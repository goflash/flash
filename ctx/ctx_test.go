package ctx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// Convenience Methods Tests

func TestRedirect(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.Redirect(http.StatusMovedPermanently, "/new-location"))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/new-location", rec.Header().Get("Location"))
	assert.True(t, c.WroteHeader())
}

func TestRedirectPermanent(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.RedirectPermanent("/permanent"))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/permanent", rec.Header().Get("Location"))
}

func TestRedirectTemporary(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.RedirectTemporary("/temporary"))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/temporary", rec.Header().Get("Location"))
}

func TestFile(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test with non-existent file
	err := c.File("non-existent-file.txt")
	assert.Error(t, err)
	assert.False(t, c.WroteHeader())
}

func TestFileFromFS(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	fs := http.Dir(".")

	// Test with non-existent file
	err := c.FileFromFS("non-existent-file.txt", fs)
	assert.Error(t, err)
	assert.False(t, c.WroteHeader())
}

func TestNotFound(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.NotFound())
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "Not Found", rec.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
}

func TestNotFoundWithMessage(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.NotFound("Custom not found message"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "Custom not found message", rec.Body.String())
}

func TestInternalServerError(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.InternalServerError())
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Internal Server Error", rec.Body.String())
}

func TestInternalServerErrorWithMessage(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.InternalServerError("Database connection failed"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Database connection failed", rec.Body.String())
}

func TestBadRequest(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.BadRequest())
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Bad Request", rec.Body.String())
}

func TestBadRequestWithMessage(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.BadRequest("Invalid JSON format"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Invalid JSON format", rec.Body.String())
}

func TestUnauthorized(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.Unauthorized())
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Unauthorized", rec.Body.String())
}

func TestUnauthorizedWithMessage(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.Unauthorized("Invalid credentials"))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Invalid credentials", rec.Body.String())
}

func TestForbidden(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.Forbidden())
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "Forbidden", rec.Body.String())
}

func TestForbiddenWithMessage(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.Forbidden("Insufficient permissions"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "Insufficient permissions", rec.Body.String())
}

func TestNoContent(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	require.NoError(t, c.NoContent())
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "", rec.Body.String())
	assert.True(t, c.WroteHeader())
}

func TestNoContentWhenHeadersAlreadyWritten(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Write headers first
	c.Header("X-Test", "value")
	c.w.WriteHeader(http.StatusOK)
	c.wroteHeader = true

	// Now call NoContent - should not change status since headers already written
	require.NoError(t, c.NoContent())
	assert.Equal(t, http.StatusOK, rec.Code) // Should remain OK, not NoContent
	assert.True(t, c.WroteHeader())
}

func TestStream(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	content := "streamed content"
	reader := strings.NewReader(content)

	require.NoError(t, c.Stream(http.StatusOK, "text/plain", reader))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, content, rec.Body.String())
	assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
	assert.Equal(t, len(content), c.wroteBytes)
}

func TestStreamWithEmptyContentType(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	content := "streamed content"
	reader := strings.NewReader(content)

	require.NoError(t, c.Stream(http.StatusOK, "", reader))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, content, rec.Body.String())
	assert.Equal(t, "", rec.Header().Get("Content-Type"))
	assert.Equal(t, len(content), c.wroteBytes)
}

func TestStreamWhenHeadersAlreadyWritten(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Write headers first
	c.Header("X-Test", "value")
	c.w.WriteHeader(http.StatusOK)
	c.wroteHeader = true

	content := "streamed content"
	reader := strings.NewReader(content)

	require.NoError(t, c.Stream(http.StatusCreated, "text/plain", reader))
	assert.Equal(t, http.StatusOK, rec.Code) // Should remain OK, not Created
	assert.Equal(t, content, rec.Body.String())
	assert.Equal(t, len(content), c.wroteBytes)
}

func TestStreamJSON(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	data := map[string]string{"message": "hello"}

	require.NoError(t, c.StreamJSON(http.StatusCreated, data))
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"message":"hello"}`, rec.Body.String())
}

func TestStreamJSONWithUnencodableData(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Create a channel which cannot be JSON encoded
	unencodable := make(chan int)

	err := c.StreamJSON(http.StatusOK, unencodable)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "json: unsupported type")
}

func TestSetCookie(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	cookie := &http.Cookie{
		Name:  "session",
		Value: "abc123",
		Path:  "/",
	}
	c.SetCookie(cookie)

	cookies := rec.Header().Values("Set-Cookie")
	assert.Len(t, cookies, 1)
	assert.Contains(t, cookies[0], "session=abc123")
}

func TestGetCookie(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", "session=abc123; user=john")
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	cookie, err := c.GetCookie("session")
	require.NoError(t, err)
	assert.Equal(t, "session", cookie.Name)
	assert.Equal(t, "abc123", cookie.Value)
}

func TestGetCookieNotFound(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	_, err := c.GetCookie("nonexistent")
	assert.Error(t, err)
}

func TestClearCookie(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	c.ClearCookie("session")

	cookies := rec.Header().Values("Set-Cookie")
	assert.Len(t, cookies, 1)
	assert.Contains(t, cookies[0], "session=")
	assert.Contains(t, cookies[0], "Max-Age=0")
	assert.Contains(t, cookies[0], "HttpOnly")
}

func TestConvenienceMethodsChaining(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test that convenience methods work with status chaining
	c.Status(http.StatusOK).Header("X-Custom", "value")
	require.NoError(t, c.NotFound("Custom message"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "Custom message", rec.Body.String())
	assert.Equal(t, "value", rec.Header().Get("X-Custom"))
}

func TestClone(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Set some state
	c.Set("key", "value")
	c.Status(http.StatusOK)

	// Clone the context
	cloned := c.Clone()

	// Verify it's a separate instance
	assert.NotSame(t, &c, cloned)

	// Verify state is copied
	assert.Equal(t, http.StatusOK, cloned.StatusCode())
	assert.Equal(t, "value", cloned.Get("key"))
}

func TestFinish(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Finish should not panic
	assert.NotPanics(t, func() {
		c.Finish()
	})
}

func TestSetRequest(t *testing.T) {
	req1, rec := newRequest(http.MethodGet, "/", nil)
	req2, _ := newRequest(http.MethodPost, "/api", nil)

	var c DefaultContext
	c.Reset(rec, req1, nil, "/")

	// Verify initial request
	assert.Equal(t, req1, c.Request())

	// Set new request
	c.SetRequest(req2)
	assert.Equal(t, req2, c.Request())
}

func TestSetResponseWriter(t *testing.T) {
	req, rec1 := newRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()

	var c DefaultContext
	c.Reset(rec1, req, nil, "/")

	// Verify initial response writer
	assert.Equal(t, rec1, c.ResponseWriter())

	// Set new response writer
	c.SetResponseWriter(rec2)
	assert.Equal(t, rec2, c.ResponseWriter())
}

func TestGetWithDefault(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	// Test getting non-existent key with default
	result := c.Get("nonexistent", "default_value")
	assert.Equal(t, "default_value", result)

	// Test getting non-existent key without default
	result = c.Get("nonexistent")
	assert.Nil(t, result)
}

func TestSet(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")

	result := c.Set("test_key", "test_value")
	assert.Equal(t, &c, result)

	assert.Equal(t, "test_value", c.Get("test_key"))
}

// Helper functions for testing
