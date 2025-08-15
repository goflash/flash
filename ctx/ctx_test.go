package ctx

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/goflash/flash/validate"
	"github.com/julienschmidt/httprouter"
	ms "github.com/mitchellh/mapstructure"
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
	var c Ctx
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
	var c Ctx
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
	var c Ctx
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
	var c Ctx
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
	var c Ctx
	c.Reset(rec, req, nil, "/")
	_, _ = c.Send(200, "", []byte("x"))
	if rec.Header().Get("Content-Length") != "1" {
		t.Fatalf("missing CL")
	}
}

func TestHeaderAndStatusCode(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c Ctx
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
	var c Ctx
	c.Reset(rec, req, ps, "/users/:id")
	assert.Equal(t, "GET", c.Method())
	assert.Equal(t, "/users/123", c.Path())
	assert.Equal(t, "/users/:id", c.Route())
	assert.Equal(t, "123", c.Param("id"))
	assert.Equal(t, "go", c.Query("q"))
}

func TestBindJSONValidAndUnknownFields(t *testing.T) {
	type In struct {
		Name string `json:"name"`
	}
	// valid
	req1, rec1 := newRequest(http.MethodPost, "/", bytes.NewBufferString("{\"name\":\"a\"}"))
	var c1 Ctx
	c1.Reset(rec1, req1, nil, "/")
	var in In
	require.NoError(t, c1.BindJSON(&in))
	assert.Equal(t, "a", in.Name)
	// unknown field should error due to DisallowUnknownFields
	req2, rec2 := newRequest(http.MethodPost, "/", bytes.NewBufferString("{\"name\":\"a\",\"x\":1}"))
	var c2 Ctx
	c2.Reset(rec2, req2, nil, "/")
	err := c2.BindJSON(&in)
	require.Error(t, err)
	assert.True(t, !errors.Is(err, io.EOF))
}

func TestJSONErrorSets500(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	type bad struct{ F func() }
	err := c.JSON(bad{})
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStatusCodeDefaultWhenHeaderNotWritten(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	c.Status(http.StatusAccepted)
	// before write, StatusCode should be 202
	assert.Equal(t, http.StatusAccepted, c.StatusCode())
}

func TestJSONSetsContentLengthAndTrimsNewline(t *testing.T) {
	req, rec := newRequest(http.MethodGet, "/", nil)
	var c Ctx
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
	var c Ctx
	c.Reset(rec, req, nil, "/p")
	if c.Request() == nil || c.ResponseWriter() == nil || c.Context() == nil {
		t.Fatalf("accessors nil")
	}
	// SetRequest/ResponseWriter
	r2 := req.Clone(req.Context())
	c.SetRequest(r2)
	c.SetResponseWriter(rec)
	_ = c.WroteHeader()
	// BindJSON unknown field error branch already tested; ensure body consumed path
	r2.Body = io.NopCloser(bytes.NewBufferString("{}"))
	c.SetRequest(r2)
	var out map[string]any
	_ = c.BindJSON(&out)
}

func TestFinishAndAccessorsCoverage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	var c Ctx
	c.Reset(rec, req, nil, "/")
	_ = c.ResponseWriter()
	_ = c.Request()
	c.Finish()
}

func TestStatusCodeBranchesV2(t *testing.T) {
	var c Ctx
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

// New tests to improve coverage for BindJSON and helpers

func TestBindJSON_TypeMismatchStrict_MapsToFieldError(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"x"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v)
	require.Error(t, err)
	fe, ok := err.(validate.FieldErrors)
	require.True(t, ok, "expected FieldErrors")
	assert.Equal(t, "int type expected", fe["age"]) // derived from struct type
}

func TestBindJSON_WeakTyping_AllowsStringToInt(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"10"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true})
	require.NoError(t, err)
	assert.Equal(t, 10, v.Age)
}

func TestBindJSON_ErrorUnused_UnknownKeysMapped(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"n","x":1}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	require.Error(t, err)
	fe, ok := err.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "unexpected", fe["x"])
}

func TestBindJSON_MapstructureTypeMismatchMapped(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, reflect.TypeOf(T{}))
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "int type expected", fe["age"]) // should use expectedTypeLabel
}

func Test_mapJSONStrictError_UnknownField(t *testing.T) {
	err := errors.New(`json: unknown field "foo"`)
	mapped := mapJSONStrictError(err, nil)
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "unexpected", fe["foo"])
}

func Test_tryJSONTypeErrorToField_WithAndWithoutTargetType(t *testing.T) {
	err := errors.New(`json: cannot unmarshal string into Go struct field User.age of type int`)
	type User struct {
		Age int `json:"age"`
	}
	mapped := tryJSONTypeErrorToField(err, reflect.TypeOf(User{}))
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "int type expected", fe["age"])
	// No target type -> generic message
	mapped2 := tryJSONTypeErrorToField(err, nil)
	fe2, ok2 := mapped2.(validate.FieldErrors)
	require.True(t, ok2)
	assert.Equal(t, "invalid type", fe2["age"])
}

func Test_tryJSONTypeErrorToField_NoMarker_ReturnsNil(t *testing.T) {
	err := errors.New("json: something else")
	mapped := tryJSONTypeErrorToField(err, nil)
	assert.Nil(t, mapped)
}

func Test_tryJSONTypeErrorToField_NoSplit_ReturnsNil(t *testing.T) {
	err := errors.New("json: cannot unmarshal string into Go struct field User.age without type")
	mapped := tryJSONTypeErrorToField(err, nil)
	assert.Nil(t, mapped)
}

func Test_tryJSONTypeErrorToField_EmptyField_ReturnsNil(t *testing.T) {
	err := errors.New("json: cannot unmarshal string into Go struct field . of type int")
	mapped := tryJSONTypeErrorToField(err, nil)
	assert.Nil(t, mapped)
}

func TestBindJSON_Strict_UnknownField_FieldErrors(t *testing.T) {
	type In struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"a","x":1}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var in In
	err := c.BindJSON(&in)
	require.Error(t, err)
	fe, ok := err.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "unexpected", fe["x"])
}

func Test_mapJSONStrictError_NoMapping_ReturnsNil(t *testing.T) {
	// An unrelated error should not map to field errors
	err := errors.New("some other error")
	mapped := mapJSONStrictError(err, nil)
	assert.Nil(t, mapped)
}

func Test_tryJSONTypeErrorToField_FieldNotFound_ReturnsInvalidType(t *testing.T) {
	err := errors.New(`json: cannot unmarshal string into Go struct field User.bogus of type int`)
	type User struct {
		Age int `json:"age"`
	}
	mapped := tryJSONTypeErrorToField(err, reflect.TypeOf(User{}))
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "invalid type", fe["bogus"]) // no matching struct field
}

func Test_mapMapstructureError_InvalidType_NoTargetType(t *testing.T) {
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, nil)
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "invalid type", fe["age"]) // no targetType provided
}

func Test_mapMapstructureError_NoConditions_ReturnsErr(t *testing.T) {
	errIn := errors.New("random parse failure")
	mapped := mapMapStructureError(errIn, BindJSONOptions{}, nil)
	assert.Equal(t, errIn, mapped)
}

func Test_extractFieldFromMapstructureTypeError_NoMatch(t *testing.T) {
	_, ok := extractFieldFromMapstructureTypeError("weird message that doesn't match")
	assert.False(t, ok)
}

func Test_extractFieldFromMapstructureTypeError_NoClosingQuote(t *testing.T) {
	_, ok := extractFieldFromMapstructureTypeError("cannot decode 'age from string into int")
	assert.False(t, ok)
}

func Test_mapMapstructureError_FieldNotFound_WithTargetType(t *testing.T) {
	type T2 struct {
		Name int `json:"name"`
	}
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, reflect.TypeOf(T2{}))
	fe, ok := mapped.(validate.FieldErrors)
	require.True(t, ok)
	assert.Equal(t, "invalid type", fe["age"])
}

func TestBindJSON_NonPointerTargetError(t *testing.T) {
	// Passing a non-pointer should result in an InvalidUnmarshalError
	body := bytes.NewBufferString("1")
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var i int
	err := c.BindJSON(i) // not a pointer
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unmarshal(non-pointer")
}

type errRC struct{}

func (e *errRC) Read(p []byte) (int, error) { return 0, errors.New("boom") }
func (e *errRC) Close() error               { return nil }

func TestBindJSON_Flexible_ReadAllError(t *testing.T) {
	// Force flexible path and make body.Read fail
	req := httptest.NewRequest(http.MethodPost, "/", &errRC{})
	rec := httptest.NewRecorder()
	var c Ctx
	c.Reset(rec, req, nil, "/")
	type T struct {
		Name string `json:"name"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestBindJSON_NonStruct_StringSuccess(t *testing.T) {
	body := bytes.NewBufferString(`"hello"`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var s string
	require.NoError(t, c.BindJSON(&s))
	assert.Equal(t, "hello", s)
}

func TestBindJSON_NonStruct_NilPointerTarget(t *testing.T) {
	body := bytes.NewBufferString(`"hello"`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var s *string // nil pointer
	err := c.BindJSON(s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unmarshal(nil")
}

func TestBindJSON_Flexible_UnmarshalError_WeakTypingTrue(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`[]`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true})
	require.Error(t, err)
	_, isFieldErr := err.(validate.FieldErrors)
	assert.False(t, isFieldErr)
	assert.Contains(t, err.Error(), "cannot unmarshal array")
}

func TestBindJSON_Flexible_WeakTypingTrue_TypeMismatch_ReturnsRaw(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	// array cannot coerce to int even with WeaklyTypedInput
	body := bytes.NewBufferString(`{"age":[1]}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true})
	require.Error(t, err)
	_, isFieldErr := err.(validate.FieldErrors)
	assert.False(t, isFieldErr)
	// error content varies; ensure it references field and type
	assert.Contains(t, err.Error(), "age")
}

func Test_expectedTypeLabel_CoversAllKinds(t *testing.T) {
	// int group
	assert.Equal(t, "int", expectedTypeLabel(reflect.TypeOf(int(0))))
	assert.Equal(t, "int", expectedTypeLabel(reflect.TypeOf(new(int)))) // pointer unwrapping
	// uint group
	assert.Equal(t, "uint", expectedTypeLabel(reflect.TypeOf(uint(0))))
	assert.Equal(t, "uint", expectedTypeLabel(reflect.TypeOf(uintptr(0))))
	// float group
	assert.Equal(t, "float", expectedTypeLabel(reflect.TypeOf(float32(0))))
	assert.Equal(t, "float", expectedTypeLabel(reflect.TypeOf(float64(0))))
	// bool
	assert.Equal(t, "bool", expectedTypeLabel(reflect.TypeOf(true)))
	// string and pointer-to-pointer string
	assert.Equal(t, "string", expectedTypeLabel(reflect.TypeOf("")))
	assert.Equal(t, "string", expectedTypeLabel(reflect.TypeOf(new(*string)))) // **string -> string
	// array and slice -> array
	assert.Equal(t, "array", expectedTypeLabel(reflect.TypeOf([1]int{})))
	assert.Equal(t, "array", expectedTypeLabel(reflect.TypeOf([]int{})))
	// map and struct -> object
	assert.Equal(t, "object", expectedTypeLabel(reflect.TypeOf(map[string]int{})))
	assert.Equal(t, "object", expectedTypeLabel(reflect.TypeOf(struct{}{})))
	// default branch (e.g., chan)
	assert.Equal(t, "chan", expectedTypeLabel(reflect.TypeOf(make(chan int))))
}

func Test_extractFieldFromMapstructureTypeError_InvalidTypePattern(t *testing.T) {
	field, ok := extractFieldFromMapstructureTypeError("invalid type for 'age' expected=int")
	require.True(t, ok)
	assert.Equal(t, "age", field)
}

func Test_extractFieldFromMapstructureTypeError_MultiLinePrefix(t *testing.T) {
	// Prefixed multi-line error message
	s := "1 error(s) decoding:\n\ninvalid type for 'age' from string into int"
	field, ok := extractFieldFromMapstructureTypeError(s)
	require.True(t, ok)
	assert.Equal(t, "age", field)
}

func Test_findExpectedFieldType_VariousBranches(t *testing.T) {
	type T struct {
		Age        int    `json:"age,omitempty"`
		Skip       string `json:"-"`
		unexported int    // intentionally unexported; should be ignored
		Name       string
	}
	// reference the unexported field to avoid unused-field linter warnings
	_ = T{}.unexported

	rt := reflect.TypeOf(T{})
	// json tag with comma option
	ft, ok := findExpectedFieldType(rt, "age")
	require.True(t, ok)
	assert.Equal(t, reflect.TypeOf(int(0)), ft)
	// '-' tag means skip
	if _, ok2 := findExpectedFieldType(rt, "skip"); ok2 {
		t.Fatalf("expected skip to be ignored")
	}
	// unexported field ignored even if tag matches
	if _, ok3 := findExpectedFieldType(rt, "secret"); ok3 {
		t.Fatalf("expected unexported field to be ignored")
	}
	// no tag -> case-insensitive match on field name
	ft2, ok4 := findExpectedFieldType(rt, "name")
	require.True(t, ok4)
	assert.Equal(t, reflect.TypeOf(""), ft2)
	// non-struct and nil
	if _, ok5 := findExpectedFieldType(nil, "age"); ok5 {
		t.Fatalf("nil type should not match")
	}
	if _, ok6 := findExpectedFieldType(reflect.TypeOf(0), "age"); ok6 {
		t.Fatalf("non-struct should not match")
	}
}

func TestBindJSON_Flexible_UnmarshalError_WeakTypingFalse_ReturnsRaw(t *testing.T) {
	type T struct {
		A int `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`[]`))
	rec := httptest.NewRecorder()
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	// Enable flexible path with ErrorUnused, but keep WeaklyTypedInput false
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal array")
}

func TestBindJSON_Flexible_WeakTypingFalse_TypeMismatch_MappedToFieldError(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"42"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c Ctx
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: false})
	require.Error(t, err)
	// When weak typing is false and a type mismatch occurs during mapstructure decode,
	// we map it to a FieldErrors value rather than returning the raw error.
	_, isFieldErr := err.(validate.FieldErrors)
	assert.True(t, isFieldErr)
}

func TestBindJSON_Flexible_JSONSyntaxError_ReturnsErr(t *testing.T) {
	req, rec := newRequest(http.MethodPost, "/", bytes.NewBufferString("{"))
	var c Ctx
	c.Reset(rec, req, nil, "/")
	type T struct {
		A int `json:"a"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	require.Error(t, err)
	e := err.Error()
	if !(strings.Contains(e, "unexpected end of JSON input") || strings.Contains(e, "unexpected EOF")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindJSON_Mapstructure_NewDecoderError(t *testing.T) {
	// Swap decoder factory to force an error
	orig := newMSDecoder
	newMSDecoder = func(c *ms.DecoderConfig) (*ms.Decoder, error) {
		return nil, errors.New("decoder boom")
	}
	defer func() { newMSDecoder = orig }()

	req, rec := newRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"x"}`))
	var c Ctx
	c.Reset(rec, req, nil, "/")
	type T struct {
		Name string `json:"name"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true}) // triggers flexible path
	require.Error(t, err)
	assert.Equal(t, "decoder boom", err.Error())
}

// New tests for Set/Get helpers
func TestCtx_SetGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	var c Ctx
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
