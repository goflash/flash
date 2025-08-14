package validate

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator is the global validator instance for goflash validation helpers.
// You can register custom tags and tag name functions on it.
var Validator = validator.New()

// messageFunc, if set by the application, converts a FieldError to a human message.
// This allows applications to plug in locale-specific translations (e.g.,
// using universal-translator and validator default translations) without the
// framework importing any locale/translation packages.
var messageFunc func(validator.FieldError) string

// SetMessageFunc sets a custom function used to render human-readable messages
// for validator.FieldError values. If not set, a small built-in fallback is used.
//
// Example (app-level):
//
//	// create translator and register default translations on validate.Validator
//	// then:
//	validate.SetMessageFunc(func(fe validator.FieldError) string { return fe.Translate(trans) })
func SetMessageFunc(fn func(validator.FieldError) string) { messageFunc = fn }

// Context key for storing a per-request message function.
type ctxKeyMsgFunc struct{}

// WithMessageFunc attaches a message function to a non-nil context and returns the derived context.
// Context must be non-nil (use c.Context() from goflash Ctx or context.Background()).
func WithMessageFunc(ctx context.Context, fn func(validator.FieldError) string) context.Context {
	return context.WithValue(ctx, ctxKeyMsgFunc{}, fn)
}

// MessageFuncFromContext retrieves a message function from context if present.
func MessageFuncFromContext(ctx context.Context) func(validator.FieldError) string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(ctxKeyMsgFunc{}).(func(validator.FieldError) string); ok {
		return v
	}
	return nil
}

func init() {
	// Use `json` tag names in error messages instead of struct field names.
	Validator.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("json")
		if name == "" || name == "-" {
			return ""
		}
		if idx := strings.Index(name, ","); idx >= 0 {
			name = name[:idx]
		}
		return name
	})
}

// Struct validates a struct using `validate` tags and the global Validator.
// Returns a ValidationErrors error if validation fails.
func Struct(s any) error { return Validator.Struct(s) }

// FieldErrors is an error type that carries a map of field->message.
// Useful for mapping JSON binding or custom validation errors to field errors.
type FieldErrors map[string]string

func (e FieldErrors) Error() string { return "field validation errors" }

// ToFieldErrors converts validator.ValidationErrors into a simple field->message map.
// If err is not a ValidationErrors, it returns a single entry under "_error".
func ToFieldErrors(err error) map[string]string { return ToFieldErrorsWith(err, messageFunc) }

// ToFieldErrorsWith is like ToFieldErrors but allows providing a custom message function
// for this call (e.g., a request-scoped translator). If fn is nil, the global SetMessageFunc
// (if any) and then the built-in fallback will be used.
func ToFieldErrorsWith(err error, fn func(validator.FieldError) string) map[string]string {
	res := map[string]string{}
	if err == nil {
		return res
	}
	if vErrs, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range vErrs {
			field := fe.Field() // uses json tag due to RegisterTagNameFunc
			if field == "" {
				field = fe.StructField()
			}
			msg := humanMessageWith(fe, fn)
			res[field] = msg
		}
		return res
	}
	// Handle custom FieldErrors directly
	if fe, ok := err.(FieldErrors); ok {
		return fe
	}
	res["_error"] = err.Error()
	return res
}

// ToFieldErrorsWithContext uses a request-scoped message function from context
// (if set via WithMessageFunc). Falls back to global SetMessageFunc and then
// built-in defaults.
func ToFieldErrorsWithContext(ctx context.Context, err error) map[string]string {
	return ToFieldErrorsWith(err, MessageFuncFromContext(ctx))
}

// humanMessage returns a message for a FieldError using the global messageFunc if set.
func humanMessage(fe validator.FieldError) string { return humanMessageWith(fe, nil) }

// humanMessageWith returns a message for a FieldError using the provided fn if not nil,
// otherwise the global messageFunc, otherwise a default fallback.
func humanMessageWith(fe validator.FieldError, fn func(validator.FieldError) string) string {
	if fn != nil {
		if msg := fn(fe); msg != "" {
			return msg
		}
	}
	if messageFunc != nil {
		if msg := messageFunc(fe); msg != "" {
			return msg
		}
	}
	return defaultMessage(fe)
}

// defaultMessage provides a minimal, dependency-free fallback for common tags.
func defaultMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "min":
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "len":
		return fmt.Sprintf("must be length %s", fe.Param())
	case "email":
		return "must be a valid email"
	case "oneof":
		return fmt.Sprintf("must be one of %s", fe.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", fe.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", fe.Param())
	case "url":
		return "must be a valid URL"
	case "uuid":
		return "must be a valid UUID"
	case "alpha":
		return "must contain only letters"
	case "alphanum":
		return "must contain only letters and numbers"
	case "numeric":
		return "must contain only numbers"
	case "contains":
		return fmt.Sprintf("must contain %s", fe.Param())
	case "excludes":
		return fmt.Sprintf("must not contain %s", fe.Param())
	case "startsWith":
		return fmt.Sprintf("must start with %s", fe.Param())
	case "endsWith":
		return fmt.Sprintf("must end with %s", fe.Param())
	case "pattern":
		return fmt.Sprintf("must match pattern %s", fe.Param())
	case "base64":
		return "must be a valid base64 string"
	case "json":
		return "must be valid JSON"
	case "ip":
		return "must be a valid IP address"
	case "cidr":
		return "must be a valid CIDR notation"
	case "time":
		return fmt.Sprintf("must be a valid time in format %s", fe.Param())
	case "timezone":
		return "must be a valid timezone"
	case "ascii":
		return "must contain only ASCII characters"
	case "printascii":
		return "must contain only printable ASCII characters"
	case "multibyte":
		return "must contain multibyte characters"
	case "iscolor":
		return "must be a valid color"
	case "isbn":
		return "must be a valid ISBN"
	case "isbn10":
		return "must be a valid ISBN-10"
	case "isbn13":
		return "must be a valid ISBN-13"
	default:
		return fmt.Sprintf("failed %s", fe.Tag())
	}
}
