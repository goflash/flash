package ctx

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	ms "github.com/mitchellh/mapstructure"
)

type userDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// helper to convert FieldErrors to map for assertions
func fieldErrorsToMap(fe FieldErrors) map[string]string {
	m := map[string]string{}
	if fe == nil {
		return m
	}
	for _, e := range fe.All() {
		m[e.Field()] = e.Message()
	}
	return m
}

func TestBindJSONValidAndUnknownFields(t *testing.T) {
	type In struct {
		Name string `json:"name"`
	}
	// valid
	req1, rec1 := newRequest(http.MethodPost, "/", bytes.NewBufferString("{\"name\":\"a\"}"))
	var c1 DefaultContext
	c1.Reset(rec1, req1, nil, "/")
	var in In
	if err := c1.BindJSON(&in); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if in.Name != "a" {
		t.Fatalf("want name=a, got %q", in.Name)
	}
	// unknown field should error due to DisallowUnknownFields
	req2, rec2 := newRequest(http.MethodPost, "/", bytes.NewBufferString("{\"name\":\"a\",\"x\":1}"))
	var c2 DefaultContext
	c2.Reset(rec2, req2, nil, "/")
	err := c2.BindJSON(&in)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestBindJSON_TypeMismatchStrict_MapsToFieldError(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"x"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v)
	if err == nil {
		t.Fatalf("expected error")
	}
	fe, ok := err.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["age"] != "int type expected" {
		t.Fatalf("wrong message: %v", m)
	}
}

func TestBindJSON_WeakTyping_AllowsStringToInt(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"10"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	if err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v.Age != 10 {
		t.Fatalf("want 10, got %d", v.Age)
	}
}

func TestBindJSON_ErrorUnused_UnknownKeysMapped(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"n","x":1}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	fe, ok := err.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["x"] != ErrFieldUnexpected.Error() {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestBindJSON_MapstructureTypeMismatchMapped(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, reflect.TypeOf(T{}))
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["age"] != "int type expected" {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_mapJSONStrictError_UnknownField(t *testing.T) {
	err := errors.New(`json: unknown field "foo"`)
	mapped := mapJSONStrictError(err, nil)
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["foo"] != ErrFieldUnexpected.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_tryJSONTypeErrorToField_WithAndWithoutTargetType(t *testing.T) {
	err := errors.New(`json: cannot unmarshal string into Go struct field User.age of type int`)
	type User struct {
		Age int `json:"age"`
	}
	mapped := tryJSONTypeErrorToField(err, reflect.TypeOf(User{}))
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["age"] != "int type expected" {
		t.Fatalf("unexpected: %#v", m)
	}
	// No target type -> generic message
	mapped2 := tryJSONTypeErrorToField(err, nil)
	fe2, ok2 := mapped2.(FieldErrors)
	if !ok2 {
		t.Fatalf("expected FieldErrors")
	}
	m2 := fieldErrorsToMap(fe2)
	if m2["age"] != ErrFieldInvalidType.Error() {
		t.Fatalf("unexpected: %#v", m2)
	}
}

func Test_tryJSONTypeErrorToField_NoMarker_ReturnsNil(t *testing.T) {
	err := errors.New("json: something else")
	if mapped := tryJSONTypeErrorToField(err, nil); mapped != nil {
		t.Fatalf("expected nil")
	}
}

func Test_tryJSONTypeErrorToField_NoSplit_ReturnsNil(t *testing.T) {
	err := errors.New("json: cannot unmarshal string into Go struct field User.age without type")
	if mapped := tryJSONTypeErrorToField(err, nil); mapped != nil {
		t.Fatalf("expected nil")
	}
}

func Test_tryJSONTypeErrorToField_EmptyField_ReturnsNil(t *testing.T) {
	err := errors.New("json: cannot unmarshal string into Go struct field . of type int")
	if mapped := tryJSONTypeErrorToField(err, nil); mapped != nil {
		t.Fatalf("expected nil")
	}
}

func TestBindJSON_Strict_UnknownField_FieldErrors(t *testing.T) {
	type In struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"a","x":1}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var in In
	err := c.BindJSON(&in)
	if err == nil {
		t.Fatalf("expected error")
	}
	fe, ok := err.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["x"] != ErrFieldUnexpected.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_mapJSONStrictError_NoMapping_ReturnsNil(t *testing.T) {
	err := errors.New("some other error")
	if mapped := mapJSONStrictError(err, nil); mapped != nil {
		t.Fatalf("expected nil")
	}
}

func Test_tryJSONTypeErrorToField_FieldNotFound_ReturnsInvalidType(t *testing.T) {
	err := errors.New(`json: cannot unmarshal string into Go struct field User.bogus of type int`)
	type User struct {
		Age int `json:"age"`
	}
	mapped := tryJSONTypeErrorToField(err, reflect.TypeOf(User{}))
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["bogus"] != ErrFieldInvalidType.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_mapMapstructureError_InvalidType_NoTargetType(t *testing.T) {
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, nil)
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["age"] != ErrFieldInvalidType.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_mapMapstructureError_NoConditions_ReturnsErr(t *testing.T) {
	errIn := errors.New("random parse failure")
	if mapped := mapMapStructureError(errIn, BindJSONOptions{}, nil); mapped != errIn {
		t.Fatalf("expected passthrough")
	}
}

func Test_extractFieldFromMapstructureTypeError_NoMatch(t *testing.T) {
	if _, ok := extractFieldFromMapStructureTypeError("weird message that doesn't match"); ok {
		t.Fatalf("expected no match")
	}
}

func Test_extractFieldFromMapstructureTypeError_NoClosingQuote(t *testing.T) {
	if _, ok := extractFieldFromMapStructureTypeError("cannot decode 'age from string into int"); ok {
		t.Fatalf("expected no match")
	}
}

func Test_mapMapstructureError_FieldNotFound_WithTargetType(t *testing.T) {
	type T2 struct {
		Name int `json:"name"`
	}
	err := errors.New("cannot decode 'age' from string into int")
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, reflect.TypeOf(T2{}))
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors")
	}
	m := fieldErrorsToMap(fe)
	if m["age"] != ErrFieldInvalidType.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestBindJSON_NonPointerTargetError(t *testing.T) {
	body := bytes.NewBufferString("1")
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var i int
	err := c.BindJSON(i)
	if err == nil || !strings.Contains(err.Error(), "Unmarshal(non-pointer") {
		t.Fatalf("expected non-pointer error, got %v", err)
	}
}

type errRC struct{}

func (e *errRC) Read(p []byte) (int, error) { return 0, errors.New("boom") }
func (e *errRC) Close() error               { return nil }

func TestBindJSON_Flexible_ReadAllError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", &errRC{})
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type T struct {
		Name string `json:"name"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestBindJSON_NonStruct_StringSuccess(t *testing.T) {
	body := bytes.NewBufferString(`"hello"`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var s string
	if err := c.BindJSON(&s); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s != "hello" {
		t.Fatalf("want hello, got %q", s)
	}
}

func TestBindJSON_NonStruct_NilPointerTarget(t *testing.T) {
	body := bytes.NewBufferString(`"hello"`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var s *string
	err := c.BindJSON(s)
	if err == nil || !strings.Contains(err.Error(), "Unmarshal(nil") {
		t.Fatalf("expected nil-pointer error, got %v", err)
	}
}

func TestBindJSON_Flexible_UnmarshalError_WeakTypingTrue(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`[]`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, isFieldErr := err.(FieldErrors); isFieldErr {
		t.Fatalf("expected raw error, got FieldErrors")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal array") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindJSON_Flexible_WeakTypingTrue_TypeMismatch_ReturnsRaw(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":[1]}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, isFieldErr := err.(FieldErrors); isFieldErr {
		t.Fatalf("expected raw error, got FieldErrors")
	}
	if !strings.Contains(err.Error(), "age") {
		t.Fatalf("expected reference to age")
	}
}

func Test_expectedTypeLabel_CoversAllKinds(t *testing.T) {
	if expectedTypeLabel(reflect.TypeOf(int(0))) != "int" {
		t.Fatal("int")
	}
	if expectedTypeLabel(reflect.TypeOf(new(int))) != "int" {
		t.Fatal("int ptr")
	}
	if expectedTypeLabel(reflect.TypeOf(uint(0))) != "uint" {
		t.Fatal("uint")
	}
	if expectedTypeLabel(reflect.TypeOf(uintptr(0))) != "uint" {
		t.Fatal("uintptr")
	}
	if expectedTypeLabel(reflect.TypeOf(float32(0))) != "float" {
		t.Fatal("float32")
	}
	if expectedTypeLabel(reflect.TypeOf(float64(0))) != "float" {
		t.Fatal("float64")
	}
	if expectedTypeLabel(reflect.TypeOf(true)) != "bool" {
		t.Fatal("bool")
	}
	if expectedTypeLabel(reflect.TypeOf("")) != "string" {
		t.Fatal("string")
	}
	var p **string
	if expectedTypeLabel(reflect.TypeOf(p)) != "string" {
		t.Fatal("**string")
	}
	if expectedTypeLabel(reflect.TypeOf([1]int{})) != "array" {
		t.Fatal("array")
	}
	if expectedTypeLabel(reflect.TypeOf([]int{})) != "array" {
		t.Fatal("slice")
	}
	if expectedTypeLabel(reflect.TypeOf(map[string]int{})) != "object" {
		t.Fatal("map")
	}
	if expectedTypeLabel(reflect.TypeOf(struct{}{})) != "object" {
		t.Fatal("struct")
	}
	if expectedTypeLabel(reflect.TypeOf(make(chan int))) != "chan" {
		t.Fatal("chan")
	}
}

func Test_extractFieldFromMapstructureTypeError_InvalidTypePattern(t *testing.T) {
	field, ok := extractFieldFromMapStructureTypeError("invalid type for 'age' expected=int")
	if !ok || field != "age" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

func Test_extractFieldFromMapstructureTypeError_MultiLinePrefix(t *testing.T) {
	s := "1 error(s) decoding:\n\ninvalid type for 'age' from string into int"
	field, ok := extractFieldFromMapStructureTypeError(s)
	if !ok || field != "age" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

func Test_findExpectedFieldType_VariousBranches(t *testing.T) {
	type T struct {
		Age        int    `json:"age,omitempty"`
		Skip       string `json:"-"`
		unexported int
		Name       string
	}
	_ = T{}.unexported
	rt := reflect.TypeOf(T{})
	if ft, ok := findExpectedFieldType(rt, "age"); !ok || ft != reflect.TypeOf(int(0)) {
		t.Fatalf("age")
	}
	if _, ok := findExpectedFieldType(rt, "skip"); ok {
		t.Fatalf("skip")
	}
	if _, ok := findExpectedFieldType(rt, "secret"); ok {
		t.Fatalf("secret")
	}
	if ft, ok := findExpectedFieldType(rt, "name"); !ok || ft != reflect.TypeOf("") {
		t.Fatalf("name")
	}
	if _, ok := findExpectedFieldType(nil, "age"); ok {
		t.Fatalf("nil")
	}
	if _, ok := findExpectedFieldType(reflect.TypeOf(0), "age"); ok {
		t.Fatalf("non-struct")
	}
}

func TestBindJSON_Flexible_UnmarshalError_WeakTypingFalse_ReturnsRaw(t *testing.T) {
	type T struct {
		A int `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`[]`))
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	if err == nil || !strings.Contains(err.Error(), "cannot unmarshal array") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBindJSON_Flexible_WeakTypingFalse_TypeMismatch_MappedToFieldError(t *testing.T) {
	type T struct {
		Age int `json:"age"`
	}
	body := bytes.NewBufferString(`{"age":"42"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var v T
	err := c.BindJSON(&v, BindJSONOptions{WeaklyTypedInput: false})
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, isFieldErr := err.(FieldErrors); !isFieldErr {
		t.Fatalf("expected FieldErrors")
	}
}

func TestBindJSON_Flexible_JSONSyntaxError_ReturnsErr(t *testing.T) {
	req, rec := newRequest(http.MethodPost, "/", bytes.NewBufferString("{"))
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type T struct {
		A int `json:"a"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	e := err.Error()
	if !(strings.Contains(e, "unexpected end of JSON input") || strings.Contains(e, "unexpected EOF")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindJSON_Mapstructure_NewDecoderError(t *testing.T) {
	orig := newMSDecoder
	newMSDecoder = func(c *ms.DecoderConfig) (*ms.Decoder, error) {
		return nil, errors.New("decoder boom")
	}
	defer func() { newMSDecoder = orig }()

	req, rec := newRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"x"}`))
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	type T struct {
		Name string `json:"name"`
	}
	var v T
	err := c.BindJSON(&v, BindJSONOptions{ErrorUnused: true})
	if err == nil || err.Error() != "decoder boom" {
		t.Fatalf("unexpected: %v", err)
	}
}

func Test_mapMapstructureError_InvalidKeys_MultiLine_ExtractsAll(t *testing.T) {
	// Simulate mapstructure multi-error with invalid keys and an additional bullet
	s := "1 error(s) decoding:\n\n* 'T' has invalid keys: extra, foo\n* 'name' expected type 'string', got unconvertible type 'float64', value: '1'\n"
	err := errors.New(s)
	mapped := mapMapStructureError(err, BindJSONOptions{ErrorUnused: true}, nil)
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors, got %T: %v", mapped, mapped)
	}
	m := fieldErrorsToMap(fe)
	if m["extra"] != ErrFieldUnexpected.Error() || m["foo"] != ErrFieldUnexpected.Error() {
		t.Fatalf("unexpected: %#v", m)
	}
}

func Test_mapMapstructureError_ExpectedType_NewPattern(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	s := "1 error(s) decoding:\n\n* 'name' expected type 'string', got unconvertible type 'float64', value: '1'\n"
	err := errors.New(s)
	mapped := mapMapStructureError(err, BindJSONOptions{WeaklyTypedInput: false}, reflect.TypeOf(T{}))
	fe, ok := mapped.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors, got %T: %v", mapped, mapped)
	}
	m := fieldErrorsToMap(fe)
	if got := m["name"]; got != "string type expected" {
		t.Fatalf("unexpected: %q (%#v)", got, m)
	}
}

func TestBindJSON_Default_ReportsAllUnexpectedFields(t *testing.T) {
	type U struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"","email":"asdf@adsf.com","extra":"unexpected field","foo":"bar"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var u U
	err := c.BindJSON(&u)
	if err == nil {
		t.Fatalf("expected error with unexpected fields")
	}
	fe, ok := err.(FieldErrors)
	if !ok {
		t.Fatalf("expected FieldErrors, got %T: %v", err, err)
	}
	m := fieldErrorsToMap(fe)
	if m["email"] != ErrFieldUnexpected.Error() || m["extra"] != ErrFieldUnexpected.Error() || m["foo"] != ErrFieldUnexpected.Error() {
		t.Fatalf("unexpected field errors: %#v", m)
	}
	if _, has := m["name"]; has {
		t.Fatalf("name should not be reported as unexpected: %#v", m)
	}
}

func TestBindJSON_ErrorUnusedFalse_IgnoresUnexpectedFields(t *testing.T) {
	type U struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"","email":"asdf@adsf.com","extra":"unexpected field","foo":"bar"}`)
	req, rec := newRequest(http.MethodPost, "/", body)
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var u U
	if err := c.BindJSON(&u, BindJSONOptions{ErrorUnused: false}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Name != "" {
		t.Fatalf("want empty name, got %q", u.Name)
	}
}

func TestBindMap_Basic(t *testing.T) {
	var c DefaultContext
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c.Reset(rec, req, nil, "/")
	m := map[string]any{"id": "42", "name": "Ann", "age": 10}
	var u userDTO
	if err := c.BindMap(&u, m, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if u.ID != "42" || u.Name != "Ann" || u.Age != 10 {
		t.Fatalf("wrong bind: %+v", u)
	}
}

func TestBindQuery(t *testing.T) {
	q := url.Values{"id": {"7"}, "name": {"Q"}, "age": {"11"}}
	u := &url.URL{Scheme: "http", Host: "ex", Path: "/", RawQuery: q.Encode()}
	req := &http.Request{Method: http.MethodGet, URL: u}
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindQuery(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "7" || out.Name != "Q" || out.Age != 11 {
		t.Fatalf("wrong: %+v", out)
	}
}

func TestBindForm(t *testing.T) {
	form := url.Values{"id": {"9"}, "name": {"F"}, "age": {"21"}}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "9" || out.Name != "F" || out.Age != 21 {
		t.Fatalf("wrong: %+v", out)
	}
}

func TestBindPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/u/xyz", nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "xyz"}, {Key: "name", Value: "P"}, {Key: "age", Value: "33"}}
	c.Reset(rec, req, ps, "/u/:id")
	var out userDTO
	if err := c.BindPath(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "xyz" || out.Name != "P" || out.Age != 33 {
		t.Fatalf("wrong: %+v", out)
	}
}

func TestBindAny_Precedence_PathOverBodyOverQuery(t *testing.T) {
	// Query lowest
	q := url.Values{"name": {"Q"}, "age": {"99"}}
	target := "/users/abc?" + q.Encode()
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"name":"J","age":"10"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "abc"}, {Key: "name", Value: "P"}}
	c.Reset(rec, req, ps, "/users/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Path name should win, age from body should override query
	if out.Name != "P" || out.Age != 10 {
		t.Fatalf("precedence wrong: %+v", out)
	}
}

// Additional coverage for BindAny with form body and precedence over query.
func TestBindAny_FormPrecedenceOverQuery(t *testing.T) {
	form := url.Values{"id": {"9"}, "name": {"F"}, "age": {"21"}}
	target := "/users/9?name=Q&age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "9"}}
	c.Reset(rec, req, ps, "/users/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Body(form) overrides query, path has only id
	if out.ID != "9" || out.Name != "F" || out.Age != 21 {
		t.Fatalf("wrong precedence: %+v", out)
	}
}

// Path should override both form and query values for the same key.
func TestBindAny_PathOverridesFormAndQuery(t *testing.T) {
	form := url.Values{"name": {"F"}, "age": {"21"}}
	target := "/users/abc?name=Q&age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "abc"}, {Key: "name", Value: "P"}}
	c.Reset(rec, req, ps, "/users/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "abc" || out.Name != "P" || out.Age != 21 { // form age overrides query
		t.Fatalf("wrong precedence: %+v", out)
	}
}

// JSON body should be ignored when Content-Type is not json; query remains.
func TestBindAny_IgnoresJSONWithoutContentType(t *testing.T) {
	body := bytes.NewBufferString(`{"age":"10","name":"J"}`)
	target := "/u/xyz?age=99&name=Q"
	req := httptest.NewRequest(http.MethodPost, target, body)
	// Intentionally no Content-Type header
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "xyz"}}
	c.Reset(rec, req, ps, "/u/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "xyz" || out.Name != "Q" || out.Age != 99 { // query wins over missing-body parse
		t.Fatalf("unexpected: %+v", out)
	}
}

// Invalid JSON with JSON content-type should return an error from BindAny.
func TestBindAny_InvalidJSON_ReturnsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/u/1", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out); err == nil {
		t.Fatalf("expected error from invalid JSON")
	}
}

// Cover valuesToMap first-value selection and mergeInto preserve flag behavior.
func Test_valuesToMap_and_mergeInto(t *testing.T) {
	vals := url.Values{"a": {"1", "2"}, "b": {"x"}}
	m := valuesToMap(vals)
	if m["a"] != "1" || m["b"] != "x" {
		t.Fatalf("valuesToMap unexpected: %#v", m)
	}

	dst := map[string]any{"a": 1, "b": 2}
	src := map[string]any{"a": 10, "c": 3}
	mergeInto(dst, src, true) // preserve existing
	if dst["a"].(int) != 1 || dst["c"].(int) != 3 {
		t.Fatalf("mergeInto preserve failed: %#v", dst)
	}
	mergeInto(dst, map[string]any{"a": 20}, false)
	if dst["a"].(int) != 20 {
		t.Fatalf("mergeInto overwrite failed: %#v", dst)
	}
}

// BindMap should allow ignoring unknown keys when ErrorUnused=false.
func TestBindMap_ErrorUnusedFalse_IgnoresUnknown(t *testing.T) {
	var c DefaultContext
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c.Reset(rec, req, nil, "/")
	in := map[string]any{"id": "1", "name": "A", "age": 7, "extra": true}
	var out userDTO
	if err := c.BindMap(&out, in, BindJSONOptions{WeaklyTypedInput: true, ErrorUnused: false}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "1" || out.Name != "A" || out.Age != 7 {
		t.Fatalf("BindMap wrong: %+v", out)
	}
}

// Vendor-specific JSON content type should be treated as JSON and override form/query where applicable.
func TestBindAny_VendorJSONContentType(t *testing.T) {
	target := "/users/1?age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"age":"10","name":"J"}`))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/users")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Age != 10 || out.Name != "J" { // JSON overrides query
		t.Fatalf("unexpected: %+v", out)
	}
}

// BindForm should surface an error for malformed form body.
func TestBindForm_InvalidForm_ReturnsError(t *testing.T) {
	body := bytes.NewBufferString("a=%zz") // invalid percent-encoding
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out); err == nil {
		t.Fatalf("expected error from invalid form encoding")
	}
}

// BindAny should surface an error when form parsing fails.
func TestBindAny_InvalidForm_ReturnsError(t *testing.T) {
	body := bytes.NewBufferString("a=%zz")
	req := httptest.NewRequest(http.MethodPost, "/u/1", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out); err == nil {
		t.Fatalf("expected error from invalid form")
	}
}

// Directly test the bullet-style error pattern in extractFieldFromMapStructureTypeError.
func Test_extractFieldFromMapstructureTypeError_BulletStyle_NewPattern(t *testing.T) {
	field, ok := extractFieldFromMapStructureTypeError("* 'name' expected type 'string', got unconvertible type 'float64', value: '1'")
	if !ok || field != "name" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

// Multipart form should be parsed by BindForm (first value per key, weak typing).
func TestBindForm_Multipart_Success(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("id", "77")
	_ = w.WriteField("name", "M")
	_ = w.WriteField("age", "31")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "77" || out.Name != "M" || out.Age != 31 {
		t.Fatalf("wrong: %+v", out)
	}
}

// BindForm should also read from a pre-populated MultipartForm.Value even without multipart body parsing.
func TestBindForm_UsesMultipartFormValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.MultipartForm = &multipart.Form{Value: map[string][]string{
		"id":   {"55"},
		"name": {"MV"},
		"age":  {"41"},
	}}
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "55" || out.Name != "MV" || out.Age != 41 {
		t.Fatalf("unexpected: %+v", out)
	}
}

// BindAny with only path parameters.
func TestBindAny_PathOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/u/xyz", nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "xyz"}, {Key: "name", Value: "P"}, {Key: "age", Value: "33"}}
	c.Reset(rec, req, ps, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "xyz" || out.Name != "P" || out.Age != 33 {
		t.Fatalf("wrong: %+v", out)
	}
}

// BindAny with only query parameters.
func TestBindAny_QueryOnly(t *testing.T) {
	q := url.Values{"id": {"7"}, "name": {"Q"}, "age": {"11"}}
	req := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "7" || out.Name != "Q" || out.Age != 11 {
		t.Fatalf("wrong: %+v", out)
	}
}

// Cover the alternate prefix in extractFieldFromMapStructureTypeError.
func Test_extractFieldFromMapstructureTypeError_AltPrefix(t *testing.T) {
	s := " error(s) decoding:\n\ninvalid type for 'age' from string into int"
	field, ok := extractFieldFromMapStructureTypeError(s)
	if !ok || field != "age" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

// Additional coverage for BindAny with form body and precedence over query.
func TestBindAny_FormPrecedenceOverQuery2(t *testing.T) {
	form := url.Values{"id": {"9"}, "name": {"F"}, "age": {"21"}}
	target := "/users/9?name=Q&age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "9"}}
	c.Reset(rec, req, ps, "/users/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Body(form) overrides query, path has only id
	if out.ID != "9" || out.Name != "F" || out.Age != 21 {
		t.Fatalf("wrong precedence: %+v", out)
	}
}

// Path should override both form and query values for the same key.
func TestBindAny_PathOverridesFormAndQuery2(t *testing.T) {
	form := url.Values{"name": {"F"}, "age": {"21"}}
	target := "/users/abc?name=Q&age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "abc"}, {Key: "name", Value: "P"}}
	c.Reset(rec, req, ps, "/users/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "abc" || out.Name != "P" || out.Age != 21 { // form age overrides query
		t.Fatalf("wrong precedence: %+v", out)
	}
}

// JSON body should be ignored when Content-Type is not json; query remains.
func TestBindAny_IgnoresJSONWithoutContentType2(t *testing.T) {
	body := bytes.NewBufferString(`{"age":"10","name":"J"}`)
	target := "/u/xyz?age=99&name=Q"
	req := httptest.NewRequest(http.MethodPost, target, body)
	// Intentionally no Content-Type header
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "xyz"}}
	c.Reset(rec, req, ps, "/u/:id")

	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "xyz" || out.Name != "Q" || out.Age != 99 { // query wins over missing-body parse
		t.Fatalf("unexpected: %+v", out)
	}
}

// Invalid JSON with JSON content-type should return an error from BindAny.
func TestBindAny_InvalidJSON_ReturnsError2(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/u/1", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out); err == nil {
		t.Fatalf("expected error from invalid JSON")
	}
}

// Cover valuesToMap first-value selection and mergeInto preserve flag behavior.
func Test_valuesToMap_and_mergeInto2(t *testing.T) {
	vals := url.Values{"a": {"1", "2"}, "b": {"x"}}
	m := valuesToMap(vals)
	if m["a"] != "1" || m["b"] != "x" {
		t.Fatalf("valuesToMap unexpected: %#v", m)
	}

	dst := map[string]any{"a": 1, "b": 2}
	src := map[string]any{"a": 10, "c": 3}
	mergeInto(dst, src, true) // preserve existing
	if dst["a"].(int) != 1 || dst["c"].(int) != 3 {
		t.Fatalf("mergeInto preserve failed: %#v", dst)
	}
	mergeInto(dst, map[string]any{"a": 20}, false)
	if dst["a"].(int) != 20 {
		t.Fatalf("mergeInto overwrite failed: %#v", dst)
	}
}

// BindMap should allow ignoring unknown keys when ErrorUnused=false.
func TestBindMap_ErrorUnusedFalse_IgnoresUnknown2(t *testing.T) {
	var c DefaultContext
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c.Reset(rec, req, nil, "/")
	in := map[string]any{"id": "1", "name": "A", "age": 7, "extra": true}
	var out userDTO
	if err := c.BindMap(&out, in, BindJSONOptions{WeaklyTypedInput: true, ErrorUnused: false}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "1" || out.Name != "A" || out.Age != 7 {
		t.Fatalf("BindMap wrong: %+v", out)
	}
}

// Vendor-specific JSON content type should be treated as JSON and override form/query where applicable.
func TestBindAny_VendorJSONContentType2(t *testing.T) {
	target := "/users/1?age=99"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"age":"10","name":"J"}`))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/users")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Age != 10 || out.Name != "J" { // JSON overrides query
		t.Fatalf("unexpected: %+v", out)
	}
}

// BindForm should surface an error for malformed form body.
func TestBindForm_InvalidForm_ReturnsError2(t *testing.T) {
	body := bytes.NewBufferString("a=%zz") // invalid percent-encoding
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out); err == nil {
		t.Fatalf("expected error from invalid form encoding")
	}
}

// BindAny should surface an error when form parsing fails.
func TestBindAny_InvalidForm_ReturnsError2(t *testing.T) {
	body := bytes.NewBufferString("a=%zz")
	req := httptest.NewRequest(http.MethodPost, "/u/1", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out); err == nil {
		t.Fatalf("expected error from invalid form")
	}
}

// Directly test the bullet-style error pattern in extractFieldFromMapStructureTypeError.
func Test_extractFieldFromMapstructureTypeError_BulletStyle_NewPattern2(t *testing.T) {
	field, ok := extractFieldFromMapStructureTypeError("* 'name' expected type 'string', got unconvertible type 'float64', value: '1'")
	if !ok || field != "name" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

// Multipart form should be parsed by BindForm (first value per key, weak typing).
func TestBindForm_Multipart_Success2(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("id", "77")
	_ = w.WriteField("name", "M")
	_ = w.WriteField("age", "31")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "77" || out.Name != "M" || out.Age != 31 {
		t.Fatalf("wrong: %+v", out)
	}
}

// BindForm should also read from a pre-populated MultipartForm.Value even without multipart body parsing.
func TestBindForm_UsesMultipartFormValue2(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.MultipartForm = &multipart.Form{Value: map[string][]string{
		"id":   {"55"},
		"name": {"MV"},
		"age":  {"41"},
	}}
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindForm(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "55" || out.Name != "MV" || out.Age != 41 {
		t.Fatalf("unexpected: %+v", out)
	}
}

// BindAny with only path parameters.
func TestBindAny_PathOnly2(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/u/xyz", nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	ps := httprouter.Params{{Key: "id", Value: "xyz"}, {Key: "name", Value: "P"}, {Key: "age", Value: "33"}}
	c.Reset(rec, req, ps, "/u/:id")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "xyz" || out.Name != "P" || out.Age != 33 {
		t.Fatalf("wrong: %+v", out)
	}
}

// BindAny with only query parameters.
func TestBindAny_QueryOnly2(t *testing.T) {
	q := url.Values{"id": {"7"}, "name": {"Q"}, "age": {"11"}}
	req := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	var c DefaultContext
	c.Reset(rec, req, nil, "/")
	var out userDTO
	if err := c.BindAny(&out, BindJSONOptions{WeaklyTypedInput: true}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != "7" || out.Name != "Q" || out.Age != 11 {
		t.Fatalf("wrong: %+v", out)
	}
}

// Cover the alternate prefix in extractFieldFromMapStructureTypeError.
func Test_extractFieldFromMapstructureTypeError_AltPrefix2(t *testing.T) {
	s := " error(s) decoding:\n\ninvalid type for 'age' from string into int"
	field, ok := extractFieldFromMapStructureTypeError(s)
	if !ok || field != "age" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}
