package ctx

import (
	"errors"
	"testing"
)

func Test_fieldError_Error_StringAndAccessors(t *testing.T) {
	fe := fieldError{field: "age", message: "invalid type"}
	if got := fe.Field(); got != "age" {
		t.Fatalf("Field() = %q", got)
	}
	if got := fe.Message(); got != "invalid type" {
		t.Fatalf("Message() = %q", got)
	}
	if got := fe.Error(); got != "field age: invalid type" {
		t.Fatalf("Error() = %q", got)
	}
}

func Test_fieldErrorsMap_Error_String(t *testing.T) {
	fe := fieldErrorsFromMap(map[string]string{"x": "unexpected"})
	if fe.Error() != "field validation errors" {
		t.Fatalf("Error() = %q", fe.Error())
	}
}

func Test_fieldErrorsMap_Is_SentinelsAndDefault(t *testing.T) {
	// Construct aggregate with a mix of messages
	fe := fieldErrorsFromMap(map[string]string{
		"x":   ErrFieldUnexpected.Error(),            // unexpected
		"age": ErrFieldInvalidType.Error(),           // invalid type
		"h":   "int " + ErrFieldTypeExpected.Error(), // "int type expected"
	})

	// Unexpected
	if !errors.Is(fe, ErrFieldUnexpected) {
		t.Fatalf("expected ErrFieldUnexpected match")
	}
	// Invalid type
	if !errors.Is(fe, ErrFieldInvalidType) {
		t.Fatalf("expected ErrFieldInvalidType match")
	}
	// Type expected (suffix match)
	if !errors.Is(fe, ErrFieldTypeExpected) {
		t.Fatalf("expected ErrFieldTypeExpected match")
	}
	// Non-sentinel target must not match
	if errors.Is(fe, errors.New("unexpected")) {
		t.Fatalf("did not expect non-sentinel match")
	}
	// Default branch: match against a custom sentinel with exact string
	// Add an extra message and verify
	fe2 := fieldErrorsFromMap(map[string]string{"k": "custom"})
	if !errors.Is(fe2, fieldSentinel("custom")) {
		t.Fatalf("expected default exact-string match")
	}
	if errors.Is(fe2, fieldSentinel("other")) {
		t.Fatalf("did not expect default mismatched string to match")
	}
}

func Test_fieldErrorsMap_All_ReturnsAll(t *testing.T) {
	fe := fieldErrorsFromMap(map[string]string{"a": "1", "b": "2"})
	got := fe.All()
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	// Order is not guaranteed; check presence
	seen := map[string]string{}
	for _, e := range got {
		seen[e.Field()] = e.Message()
	}
	if seen["a"] != "1" || seen["b"] != "2" {
		t.Fatalf("unexpected contents: %#v", seen)
	}
}

func Test_fieldErrorsFromMap_EmptyIsNil(t *testing.T) {
	if got := fieldErrorsFromMap(map[string]string{}); got != nil {
		t.Fatalf("expected nil for empty map")
	}
}
