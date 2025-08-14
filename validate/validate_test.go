package validate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	validator "github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

type user struct {
	Name string `json:"name" validate:"required,min=2"`
	Age  int    `json:"age" validate:"min=0,max=130"`
}

func TestValidateStructAndToFieldErrors(t *testing.T) {
	u := user{Name: "a", Age: -1}
	if err := Struct(u); err == nil {
		t.Fatalf("expected error")
	}
	u = user{Name: "ab", Age: 10}
	if err := Struct(u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToFieldErrors(t *testing.T) {
	u := user{Name: "a", Age: -1}
	err := Struct(u)
	m := ToFieldErrors(err)
	if len(m) == 0 {
		t.Fatalf("expected field errors")
	}
	if _, ok := m["name"]; !ok {
		t.Fatalf("expected name error mapped")
	}
}

func TestToFieldErrorsWithNonValidationError(t *testing.T) {
	m := ToFieldErrors(assert.AnError)
	if _, ok := m["_error"]; !ok {
		t.Fatalf("expected _error entry")
	}
}

func TestFieldErrorsError(t *testing.T) {
	var err error = FieldErrors{"name": "is required"}
	if err.Error() != "field validation errors" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}

func TestToFieldErrorsNilReturnsEmpty(t *testing.T) {
	m := ToFieldErrors(nil)
	if len(m) != 0 {
		t.Fatalf("expected empty map for nil error, got %v", m)
	}
}

func TestToFieldErrorsFieldErrorsPassthrough(t *testing.T) {
	in := FieldErrors{"foo": "bar"}
	m := ToFieldErrors(in)
	if len(m) != 1 || m["foo"] != "bar" {
		t.Fatalf("expected passthrough FieldErrors, got %v", m)
	}
}

func TestToFieldErrorsFallbackToStructField(t *testing.T) {
	type S struct {
		Secret string `json:"-" validate:"required"`
	}
	err := Struct(S{}) // missing Secret triggers validation error
	m := ToFieldErrors(err)
	// When tag name func returns empty for json:"-", we should fallback to StructField()
	if _, ok := m["Secret"]; !ok {
		t.Fatalf("expected fallback to StructField key 'Secret', got %v", m)
	}
}

func TestSetMessageFuncOverridesDefault(t *testing.T) {
	// Ensure we reset global after test
	defer SetMessageFunc(nil)
	SetMessageFunc(func(fe validator.FieldError) string { return "OVERRIDDEN" })
	u := user{Name: "a", Age: -1}
	err := Struct(u)
	m := ToFieldErrors(err)
	if m["name"] != "OVERRIDDEN" {
		t.Fatalf("expected overridden message, got %v", m["name"])
	}
}

func TestMessageFuncFromContext_Nil(t *testing.T) {
	if MessageFuncFromContext(nil) != nil {
		t.Fatalf("expected nil when ctx is nil")
	}
}

func TestMessageFuncFromContext_NoValue(t *testing.T) {
	if MessageFuncFromContext(context.Background()) != nil {
		t.Fatalf("expected nil when no message func in context")
	}
}

func TestMessageFuncFromContext_WithValue(t *testing.T) {
	ctx := WithMessageFunc(context.Background(), func(fe validator.FieldError) string { return "CTX" })
	fn := MessageFuncFromContext(ctx)
	if fn == nil {
		t.Fatalf("expected non-nil message func from context")
	}
	// Ensure ToFieldErrorsWithContext uses the context function
	u := user{} // Name required -> triggers error
	err := Struct(u)
	m := ToFieldErrorsWithContext(ctx, err)
	assert.Equal(t, "CTX", m["name"])
}

func TestDefaultMessage_ManyTags(t *testing.T) {
	// Ensure default messages are used
	defer SetMessageFunc(nil)

	type DM struct {
		Req        string `json:"req" validate:"required"`
		Min        string `json:"min" validate:"min=3"`
		Max        string `json:"max" validate:"max=5"`
		Len        string `json:"len" validate:"len=2"`
		Email      string `json:"email" validate:"email"`
		One        string `json:"one" validate:"oneof=a b c"`
		GTE        int    `json:"gte" validate:"gte=5"`
		LTE        int    `json:"lte" validate:"lte=3"`
		URL        string `json:"url" validate:"url"`
		UUID       string `json:"uuid" validate:"uuid"`
		Alpha      string `json:"alpha" validate:"alpha"`
		Alphanum   string `json:"alphanum" validate:"alphanum"`
		Numeric    string `json:"numeric" validate:"numeric"`
		Contains   string `json:"contains" validate:"contains=x"`
		Excludes   string `json:"excludes" validate:"excludes=y"`
		Base64     string `json:"base64" validate:"base64"`
		JSON       string `json:"json" validate:"json"`
		IP         string `json:"ip" validate:"ip"`
		CIDR       string `json:"cidr" validate:"cidr"`
		ASCII      string `json:"ascii" validate:"ascii"`
		PrintASCII string `json:"printascii" validate:"printascii"`
		Multibyte  string `json:"multibyte" validate:"multibyte"`
		IsColor    string `json:"iscolor" validate:"iscolor"`
		ISBN       string `json:"isbn" validate:"isbn"`
		ISBN10     string `json:"isbn10" validate:"isbn10"`
		ISBN13     string `json:"isbn13" validate:"isbn13"`
	}

	bad := DM{
		Req:        "",
		Min:        "ab",
		Max:        "abcdef",
		Len:        "a",
		Email:      "not-an-email",
		One:        "z",
		GTE:        3,
		LTE:        5,
		URL:        "not-a-url",
		UUID:       "x",
		Alpha:      "abc123",
		Alphanum:   "ab!",
		Numeric:    "abc",
		Contains:   "abc",
		Excludes:   "has y inside",
		Base64:     "****",
		JSON:       "{",
		IP:         "999.0.0.1",
		CIDR:       "999.0.0.1/33",
		ASCII:      "✓",
		PrintASCII: "a" + string(rune(7)),
		Multibyte:  "abc",
		IsColor:    "not-a-color",
		ISBN:       "foo",
		ISBN10:     "123",
		ISBN13:     "123",
	}

	err := Struct(bad)
	if err == nil {
		t.Fatalf("expected validation errors")
	}
	m := ToFieldErrors(err)
	assert.Equal(t, "is required", m["req"])
	assert.Equal(t, "must be at least 3", m["min"])
	assert.Equal(t, "must be at most 5", m["max"])
	assert.Equal(t, "must be length 2", m["len"])
	assert.Equal(t, "must be a valid email", m["email"])
	assert.Equal(t, "must be one of a b c", m["one"])
	assert.Equal(t, "must be greater than or equal to 5", m["gte"])
	assert.Equal(t, "must be less than or equal to 3", m["lte"])
	assert.Equal(t, "must be a valid URL", m["url"])
	assert.Equal(t, "must be a valid UUID", m["uuid"])
	assert.Equal(t, "must contain only letters", m["alpha"])
	assert.Equal(t, "must contain only letters and numbers", m["alphanum"])
	assert.Equal(t, "must contain only numbers", m["numeric"])
	assert.Equal(t, "must contain x", m["contains"])
	assert.Equal(t, "must not contain y", m["excludes"])
	assert.Equal(t, "must be a valid base64 string", m["base64"])
	assert.Equal(t, "must be valid JSON", m["json"])
	assert.Equal(t, "must be a valid IP address", m["ip"])
	assert.Equal(t, "must be a valid CIDR notation", m["cidr"])
	assert.Equal(t, "must contain only ASCII characters", m["ascii"])
	assert.Equal(t, "must contain only printable ASCII characters", m["printascii"])
	assert.Equal(t, "must contain multibyte characters", m["multibyte"])
	assert.Equal(t, "must be a valid color", m["iscolor"])
	assert.Equal(t, "must be a valid ISBN", m["isbn"])
	assert.Equal(t, "must be a valid ISBN-10", m["isbn10"])
	assert.Equal(t, "must be a valid ISBN-13", m["isbn13"])
}

type allTags struct {
	Req   string `json:"req" validate:"required"`
	Min   string `json:"min" validate:"min=2"`
	Max   string `json:"max" validate:"max=3"`
	Len   string `json:"len" validate:"len=2"`
	Email string `json:"email" validate:"email"`
	OneOf string `json:"one" validate:"oneof=a b"`
}

type dashTag struct {
	Field string `json:"-" validate:"required"`
}

type customTag struct {
	F string `json:"f" validate:"iscolor"`
}

func TestHumanMessageBranchesViaValidation(t *testing.T) {
	v := validator.New()
	// use json tag names
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("json")
		if idx := strings.Index(name, ","); idx >= 0 {
			name = name[:idx]
		}
		return name
	})
	a := allTags{Req: "", Min: "a", Max: "abcd", Len: "a", Email: "x", OneOf: "z"}
	err := v.Struct(a)
	ves, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected validation errors")
	}
	// Map with our human messages
	for _, fe := range ves {
		_ = humanMessage(fe)
	}
	m := ToFieldErrors(err)
	if len(m) == 0 {
		t.Fatalf("expected mapped errors")
	}
}

func TestToFieldErrorsNonValidation(t *testing.T) {
	m := ToFieldErrors(errors.New("boom"))
	if m["_error"] == "" {
		t.Fatalf("expected _error mapping")
	}
}

func TestValidatorInitTagNameDash(t *testing.T) {
	// ensure package-level Validator tag name func removes '-' fields
	d := dashTag{}
	err := Struct(d) // uses package Validator from init()
	if err == nil {
		t.Fatalf("expected error")
	}
	m := ToFieldErrors(err)
	if got, ok := m["Field"]; !ok || got != "is required" {
		t.Fatalf("expected fallback key 'Field' with message 'is required', got: %#v", m)
	}
}

func TestHumanMessageDefaultBranch(t *testing.T) {
	v := validator.New()
	v.RegisterValidation("iscolor", func(fl validator.FieldLevel) bool { return false })
	v.RegisterTagNameFunc(func(fld reflect.StructField) string { return fld.Tag.Get("json") })
	u := customTag{F: "xxx"}
	err := v.Struct(u)
	ves := err.(validator.ValidationErrors)
	got := humanMessage(ves[0])
	if got == "" || got == "is required" {
		t.Fatalf("expected default message, got %q", got)
	}
}

type noJSONTag struct {
	Name string `validate:"required"`
}

func TestToFieldErrorsFallsBackToStructField(t *testing.T) {
	u := noJSONTag{}
	err := Struct(u)
	m := ToFieldErrors(err)
	if _, ok := m["Name"]; !ok {
		t.Fatalf("expected StructField fallback key, got: %#v", m)
	}
}

func TestToFieldErrorsNil(t *testing.T) {
	m := ToFieldErrors(nil)
	if len(m) != 0 {
		t.Fatalf("expected empty map for nil error")
	}
}

func TestToFieldErrorsNonValidationDup(t *testing.T) {
	m := ToFieldErrors(errors.New("x"))
	if m["_error"] == "" {
		t.Fatalf("expected _error")
	}
}

type commaJSON struct {
	Field string `json:"pretty_name,omitempty" validate:"required"`
}

func TestValidatorInitTagNameCommaStripsOptions(t *testing.T) {
	// Uses package-level Validator via Struct()
	var v commaJSON
	err := Struct(v)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	m := ToFieldErrors(err)
	if _, ok := m["pretty_name"]; !ok {
		t.Fatalf("expected key 'pretty_name', got: %#v", m)
	}
}

func TestToFieldErrorsDashJSONFallbackAndMessages(t *testing.T) {
	obj := struct {
		Hidden string `json:"-" validate:"required"`
	}{}
	err := Struct(obj)
	m := ToFieldErrors(err)
	if got, ok := m["Hidden"]; !ok || got != "is required" {
		t.Fatalf("expected fallback 'Hidden' -> 'is required', got: %#v", m)
	}
}

type ctxUser struct {
	Name string `json:"name" validate:"required,min=2"`
}

func TestToFieldErrorsWithContext_UsesRequestScopedFunc(t *testing.T) {
	defer SetMessageFunc(nil) // ensure global is cleared
	// Build a validation error
	var u ctxUser
	err := Struct(u)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	// Provide request-scoped translator via context
	ctx := WithMessageFunc(context.Background(), func(fe validator.FieldError) string { return "CTXMSG" })
	m := ToFieldErrorsWithContext(ctx, err)
	if got := m["name"]; got != "CTXMSG" {
		t.Fatalf("expected CTXMSG, got %q (map=%v)", got, m)
	}
}

func TestToFieldErrorsWithContext_FallsBackToGlobal(t *testing.T) {
	defer SetMessageFunc(nil)
	SetMessageFunc(func(fe validator.FieldError) string { return "GLOBALMSG" })
	var u ctxUser
	err := Struct(u)
	m := ToFieldErrorsWithContext(context.Background(), err)
	if got := m["name"]; got != "GLOBALMSG" {
		t.Fatalf("expected GLOBALMSG, got %q (map=%v)", got, m)
	}
}

func TestMessageFuncFromContext_Present(t *testing.T) {
	mfExp := func(fe validator.FieldError) string { return "X" }
	ctx := WithMessageFunc(context.TODO(), mfExp)
	mf := MessageFuncFromContext(ctx)
	if mf == nil || mf(nil) != "X" {
		t.Fatalf("expected message function from context to work")
	}
}

func TestWithMessageFunc_NonNilContext(t *testing.T) {
	ctx := WithMessageFunc(context.Background(), func(fe validator.FieldError) string { return "ok" })
	if mf := MessageFuncFromContext(ctx); mf == nil || mf(nil) != "ok" {
		t.Fatalf("expected stored message function to be retrievable")
	}
}

func TestToFieldErrorsWithContext_EmptyFn_FallsBackToGlobal(t *testing.T) {
	defer SetMessageFunc(nil)
	SetMessageFunc(func(fe validator.FieldError) string { return "GLOBAL_FALLBACK" })
	// Build a validation error
	type X struct {
		A string `json:"a" validate:"required"`
	}
	err := Struct(X{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	// Provide request-scoped fn that returns empty string -> should fallback to global
	ctx := WithMessageFunc(context.Background(), func(fe validator.FieldError) string { return "" })
	m := ToFieldErrorsWithContext(ctx, err)
	if got := m["a"]; got != "GLOBAL_FALLBACK" {
		t.Fatalf("expected GLOBAL_FALLBACK, got %q (map=%v)", got, m)
	}
}

func TestToFieldErrorsWithContext_EmptyFn_DefaultFallback(t *testing.T) {
	defer SetMessageFunc(nil)
	// Ensure no global message func
	SetMessageFunc(nil)
	// Build a validation error
	type X struct {
		A string `json:"a" validate:"required"`
	}
	err := Struct(X{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	// Provide request-scoped fn that returns empty string -> should fallback to defaultMessage
	ctx := WithMessageFunc(context.Background(), func(fe validator.FieldError) string { return "" })
	m := ToFieldErrorsWithContext(ctx, err)
	if got := m["a"]; got != "is required" {
		t.Fatalf("expected default 'is required', got %q (map=%v)", got, m)
	}
}

func TestToFieldErrorsWith_Direct_NonValidationError(t *testing.T) {
	m := ToFieldErrorsWith(assert.AnError, nil)
	if got := m["_error"]; got != assert.AnError.Error() {
		t.Fatalf("expected _error mapping, got %q", got)
	}
}

func TestToFieldErrorsWith_Direct_FieldErrorsPassthrough(t *testing.T) {
	in := FieldErrors{"a": "b"}
	m := ToFieldErrorsWith(in, nil)
	if len(m) != 1 || m["a"] != "b" {
		t.Fatalf("expected passthrough, got %#v", m)
	}
}

func TestToFieldErrorsWith_Direct_NilError(t *testing.T) {
	m := ToFieldErrorsWith(nil, nil)
	if len(m) != 0 {
		t.Fatalf("expected empty map for nil error, got %#v", m)
	}
}

func TestToFieldErrorsWith_CustomFnWins(t *testing.T) {
	// Build a validation error
	type X struct {
		A string `json:"a" validate:"required"`
	}
	err := Struct(X{})
	m := ToFieldErrorsWith(err, func(fe validator.FieldError) string { return "CUSTOM" })
	if got := m["a"]; got != "CUSTOM" {
		t.Fatalf("expected CUSTOM, got %q (map=%v)", got, m)
	}
}

func TestToFieldErrorsWith_DashTag_FallbackKey(t *testing.T) {
	type D struct {
		Hidden string `json:"-" validate:"required"`
	}
	err := Struct(D{})
	m := ToFieldErrorsWith(err, nil)
	if got, ok := m["Hidden"]; !ok || got == "" {
		t.Fatalf("expected fallback StructField key 'Hidden', got: %#v", m)
	}
}
