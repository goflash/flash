package ctx

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	ms "github.com/mitchellh/mapstructure"
)

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
	if _, ok := extractFieldFromMapstructureTypeError("weird message that doesn't match"); ok {
		t.Fatalf("expected no match")
	}
}

func Test_extractFieldFromMapstructureTypeError_NoClosingQuote(t *testing.T) {
	if _, ok := extractFieldFromMapstructureTypeError("cannot decode 'age from string into int"); ok {
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
	field, ok := extractFieldFromMapstructureTypeError("invalid type for 'age' expected=int")
	if !ok || field != "age" {
		t.Fatalf("unexpected: %v %v", field, ok)
	}
}

func Test_extractFieldFromMapstructureTypeError_MultiLinePrefix(t *testing.T) {
	s := "1 error(s) decoding:\n\ninvalid type for 'age' from string into int"
	field, ok := extractFieldFromMapstructureTypeError(s)
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
