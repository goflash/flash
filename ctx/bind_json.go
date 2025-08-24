package ctx

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"

	ms "github.com/mitchellh/mapstructure"
)

// newMSDecoder is a package-level hook to allow tests to stub mapstructure decoder creation.
var newMSDecoder = ms.NewDecoder

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
func (c *DefaultContext) BindJSON(v any, opts ...BindJSONOptions) error {
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

// mapJSONStrictError converts encoding/json errors into field errors when possible.
func mapJSONStrictError(err error, targetType reflect.Type) error {
	s := err.Error()
	// Unknown field: "json: unknown field \"asdf\""
	if strings.Contains(s, "unknown field ") {
		start := strings.Index(s, "\"")
		if start != -1 {
			end := strings.Index(s[start+1:], "\"")
			if end != -1 {
				field := s[start+1 : start+1+end]
				if field != "" {
					return fieldErrorsFromMap(map[string]string{field: ErrFieldUnexpected.Error()})
				}
			}
		}
	}
	// Type mismatch -> look up expected type from struct and report it
	if fErr := tryJSONTypeErrorToField(err, targetType); fErr != nil {
		return fErr
	}
	return nil
}

// tryJSONTypeErrorToField attempts to convert a stdlib json type error into FieldErrors.
func tryJSONTypeErrorToField(err error, targetType reflect.Type) error {
	s := err.Error()
	// Look for "Go struct field <Type>.<field> of type <type>"
	const marker = "Go struct field "
	i := strings.Index(s, marker)
	if i == -1 {
		return nil
	}
	s = s[i+len(marker):]
	// Now s like: User.age of type int
	parts := strings.Split(s, " of type ")
	if len(parts) != 2 {
		return nil
	}
	fieldPath := parts[0]
	// fieldPath like: User.age
	if idx := strings.LastIndex(fieldPath, "."); idx != -1 {
		fieldPath = fieldPath[idx+1:]
	}
	if fieldPath == "" {
		return nil
	}
	// Derive expected type label from struct if available
	if targetType != nil && targetType.Kind() == reflect.Struct {
		if ft, ok := findExpectedFieldType(targetType, fieldPath); ok {
			return fieldErrorsFromMap(map[string]string{fieldPath: expectedTypeLabel(ft) + " " + ErrFieldTypeExpected.Error()})
		}
	}
	return fieldErrorsFromMap(map[string]string{fieldPath: ErrFieldInvalidType.Error()})
}

// mapMapStructureError converts mapstructure errors into FieldErrors with friendly messages.
func mapMapStructureError(err error, o BindJSONOptions, targetType reflect.Type) error {
	// mapstructure may return a multi-error string; handle key cases.
	s := err.Error()
	// Unknown field when ErrorUnused is true: "has invalid keys: asdf, ..."
	if o.ErrorUnused {
		if strings.Contains(s, "has invalid keys:") {
			// Extract keys substring after the specific marker, not just the first colon in the string
			marker := "has invalid keys:"
			idx := strings.Index(s, marker)
			if idx != -1 {
				list := s[idx+len(marker):]
				// Normalize whitespace/newlines
				list = strings.TrimSpace(list)
				// Split by comma and trim punctuation/whitespace around keys
				parts := strings.Split(list, ",")
				fe := map[string]string{}
				for _, p := range parts {
					k := strings.TrimSpace(p)
					// strip trailing punctuation if present
					k = strings.Trim(k, " .;:")
					if k != "" {
						fe[k] = ErrFieldUnexpected.Error()
					}
				}
				if len(fe) > 0 {
					return fieldErrorsFromMap(fe)
				}
			}
		}
	}
	// Type mismatch when WeaklyTypedInput is false. mapstructure reports e.g.:
	// "cannot decode 'age' from string into int"
	if !o.WeaklyTypedInput {
		if field, ok := extractFieldFromMapstructureTypeError(s); ok {
			if targetType != nil {
				if ft, ok2 := findExpectedFieldType(targetType, field); ok2 {
					return fieldErrorsFromMap(map[string]string{field: expectedTypeLabel(ft) + " " + ErrFieldTypeExpected.Error()})
				}
			}
			return fieldErrorsFromMap(map[string]string{field: ErrFieldInvalidType.Error()})
		}
	}
	return err
}

// extractFieldFromMapstructureTypeError extracts the field name from a mapstructure type error string.
func extractFieldFromMapstructureTypeError(s string) (string, bool) {
	if strings.HasPrefix(s, "1 error(s) decoding:") {
		lines := strings.Split(s, "\n")
		s = strings.TrimSpace(lines[len(lines)-1])
	}
	start := strings.Index(s, "cannot decode '")
	if start == -1 {
		start = strings.Index(s, "invalid type for '")
		if start == -1 {
			return "", false
		}
		start += len("invalid type for '")
	} else {
		start += len("cannot decode '")
	}
	end := strings.Index(s[start:], "'")
	if end == -1 {
		return "", false
	}
	field := s[start : start+end]
	return field, true
}

// findExpectedFieldType finds the struct field type by matching json tag name (or field name if no tag).
func findExpectedFieldType(t reflect.Type, jsonField string) (reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Tag.Get("json")
		if name != "" {
			if idx := strings.Index(name, ","); idx >= 0 {
				name = name[:idx]
			}
			if name == "-" {
				continue
			}
			if strings.EqualFold(name, jsonField) {
				return f.Type, true
			}
		}
		// No json tag: case-insensitive match on field name
		if strings.EqualFold(f.Name, jsonField) {
			return f.Type, true
		}
	}
	return nil, false
}

func expectedTypeLabel(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.String:
		return "string"
	case reflect.Array, reflect.Slice:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return t.Kind().String()
	}
}
