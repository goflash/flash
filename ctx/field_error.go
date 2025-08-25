package ctx

import (
	"fmt"
	"strings"
)

// fieldSentinel is a light-weight error used for sentinel comparisons.
//
// It allows FieldErrors to participate in errors.Is comparisons without
// exposing internal implementation details. Users typically won't interact
// with this type directly; instead they should use ErrFieldUnexpected,
// ErrFieldInvalidType, or ErrFieldTypeExpected.
type fieldSentinel string

func (e fieldSentinel) Error() string { return string(e) }

// Sentinel errors to detect common field error categories with errors.Is.
//
// These values enable ergonomic detection of field-level validation/binding
// issues returned by helpers (e.g., BindJSON, BindMap, BindAny). The messages
// of generated FieldErrors remain human-friendly (e.g., "unexpected",
// "invalid type", "int type expected"), while errors.Is can be used to check
// categories.
//
// Example (categorizing field errors):
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) {
//	    if errors.Is(fe, ctx.ErrFieldUnexpected) {
//	        // Unknown input keys were present
//	    }
//	    if errors.Is(fe, ctx.ErrFieldTypeExpected) {
//	        // At least one field has a precise expected-type message
//	    }
//	}
//
// Example (surface messages to clients):
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) {
//	    out := map[string]string{}
//	    for _, e := range fe.All() {
//	        out[e.Field()] = e.Message() // e.g., {"age":"int type expected"}
//	    }
//	    _ = out
//	}
var (
	// ErrFieldUnexpected matches unknown/unexpected input fields.
	ErrFieldUnexpected error = fieldSentinel("unexpected")
	// ErrFieldInvalidType matches type mismatches without a known expected type.
	ErrFieldInvalidType error = fieldSentinel("invalid type")
	// ErrFieldTypeExpected matches any message that ends with " type expected" (e.g., "int type expected").
	ErrFieldTypeExpected error = fieldSentinel("type expected")
)

// FieldError represents a validation or binding error for a specific field.
// Implementations provide a field path/name and a human-friendly message.
// The same information can be obtained via Error(), but Field()/Message() are
// convenient for structured handling in application code and for serializing
// field-specific errors in APIs.
//
// Example (printing structured errors):
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) {
//	    for _, e := range fe.All() {
//	        log.Printf("%s -> %s", e.Field(), e.Message())
//	    }
//	}
type FieldError interface {
	Field() string
	Message() string
}

// FieldErrors represents multiple field validation/binding errors for a single
// decoding/binding operation.
//
// FieldErrors satisfies the error interface. It also implements Is to support
// errors.Is comparisons against the sentinel errors in this package.
//
// Example (group handling and iteration):
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) {
//	    if errors.Is(fe, ctx.ErrFieldUnexpected) {
//	        // handle unexpected input fields collectively
//	    }
//	    for _, e := range fe.All() {
//	        fmt.Println(e.Error()) // "field <name>: <message>"
//	    }
//	}
type FieldErrors interface {
	error
	All() []FieldError
}

// concrete implementations
type fieldError struct {
	field   string
	message string
}

func (e fieldError) Field() string   { return e.field }
func (e fieldError) Message() string { return e.message }
func (e fieldError) Error() string   { return fmt.Sprintf("field %s: %s", e.field, e.message) }

type fieldErrorsMap struct {
	m map[string]string
}

func (f fieldErrorsMap) Error() string {
	return "field validation errors"
}

// Is enables errors.Is to detect sentinel field error categories on the aggregate.
// It matches true if any contained field error belongs to the requested category.
// This powers expressions such as errors.Is(err, ErrFieldUnexpected) when err is
// or wraps a FieldErrors value.
//
// Example:
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) && errors.Is(fe, ctx.ErrFieldInvalidType) {
//	    // At least one field had an invalid type
//	}
func (f fieldErrorsMap) Is(target error) bool {
	// We match only against our sentinel type to avoid accidental string matches.
	s, ok := target.(fieldSentinel)
	if !ok {
		return false
	}
	for _, msg := range f.m {
		switch s {
		case ErrFieldTypeExpected.(fieldSentinel):
			if strings.HasSuffix(msg, " "+ErrFieldTypeExpected.Error()) {
				return true
			}
		case ErrFieldUnexpected.(fieldSentinel):
			if msg == ErrFieldUnexpected.Error() {
				return true
			}
		case ErrFieldInvalidType.(fieldSentinel):
			if msg == ErrFieldInvalidType.Error() {
				return true
			}
		default:
			if msg == s.Error() {
				return true
			}
		}
	}
	return false
}

// All returns the list of individual field errors contained in the aggregate.
// Each entry exposes the field path/name and a human-friendly message.
// The order is unspecified; callers should not rely on ordering semantics.
//
// Example:
//
//	var fe ctx.FieldErrors
//	if errors.As(err, &fe) {
//	    errs := map[string]string{}
//	    for _, e := range fe.All() {
//	        errs[e.Field()] = e.Message()
//	    }
//	    _ = errs
//	}
func (f fieldErrorsMap) All() []FieldError {
	out := make([]FieldError, 0, len(f.m))
	for k, v := range f.m {
		out = append(out, fieldError{field: k, message: v})
	}
	return out
}

// fieldErrorsFromMap constructs a FieldErrors aggregate from field->message
// pairs. If the provided map is empty, it returns nil.
//
// This helper is used internally by binders to create structured field errors
// while preserving simple, readable messages for end users.
//
// Example (internal-style usage):
//
//	return fieldErrorsFromMap(map[string]string{"age": "int type expected"})
func fieldErrorsFromMap(m map[string]string) FieldErrors {
	if len(m) == 0 {
		return nil
	}
	return fieldErrorsMap{m: m}
}
