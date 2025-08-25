package ctx

import (
	"encoding/json"
	"mime"
	"net/url"
	"reflect"
	"strings"

	ms "github.com/mitchellh/mapstructure"
)

// newMSDecoder is a package-level hook to allow tests to stub map structure decoder creation.
var newMSDecoder = ms.NewDecoder

// BindJSONOptions customizes how JSON and map binding decode payloads into structs.
//
// Defaults when options are omitted:
//   - ErrorUnused = true  (unknown fields cause an error)
//   - WeaklyTypedInput = false (no implicit type coercion)
//
// If an options value is provided explicitly, its zero-values are honored as-is.
//
// Example (strict decoding, reject unknown fields):
//
//	type User struct {
//		ID   int    `json:"id"`
//		Name string `json:"name"`
//	}
//
//	var u User
//	err := c.BindJSON(&u) // same as c.BindJSON(&u, BindJSONOptions{ErrorUnused: true})
//	if fe, ok := ctx.AsFieldErrors(err); ok {
//		// fe["extra"] == "unexpected field" when body contains unknown key "extra"
//	}
//
// Example (allow coercion, allow unknown fields):
//
//	var u User
//	_ = c.BindJSON(&u, BindJSONOptions{WeaklyTypedInput: true, ErrorUnused: false})
type BindJSONOptions struct {
	// WeaklyTypedInput allows common type coercions, e.g., "10" -> 10 for int fields.
	WeaklyTypedInput bool
	// ErrorUnused when true returns an error for unexpected fields.
	ErrorUnused bool
}

// BindJSON decodes the request body JSON into v.
//
// When v is a pointer to a struct, you may pass BindJSONOptions to control strictness
// and coercion; otherwise, non-struct targets use the standard library's strict
// behavior with DisallowUnknownFields.
//
// Default behavior (no options):
//   - Unknown fields are reported as field errors
//   - No weak typing (strings are not coerced into numbers, etc.)
//
// Field error mapping: common json.Decoder errors are converted into user-friendly
// FieldErrors keyed by the offending json field.
//
// Examples:
//
//	// 1) Strict struct binding
//	type Payload struct {
//		Age int `json:"age"`
//	}
//	var p Payload
//	if err := c.BindJSON(&p); err != nil {
//		// Unknown fields => field error: {"extra": "unexpected field"}
//	}
//
//	// 2) Permissive binding (coercion + allow unknown)
//	_ = c.BindJSON(&p, BindJSONOptions{WeaklyTypedInput: true, ErrorUnused: false})
//
//	// 3) Non-struct target (map or slice)
//	var m map[string]any
//	_ = c.BindJSON(&m) // uses DisallowUnknownFields and returns raw json errors
func (c *DefaultContext) BindJSON(v any, opts ...BindJSONOptions) error {
	// Non-struct targets: keep strict json decoder behavior regardless of options.
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		defer c.r.Body.Close()
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
	// For struct targets, collect to map and delegate to BindMap for consistent behavior.
	m, err := c.collectJSONMap()
	if err != nil {
		return err
	}
	return c.BindMap(v, m, opts...)
}

// BindMap binds fields from the provided map into v using mapstructure, honoring options.
// TagName is "json" for all binders to keep a single source-of-truth for names.
//
// The map's keys must match the struct's `json` tag names (or field names if tag missing).
// Type conversion behavior is governed by BindJSONOptions.WeaklyTypedInput.
// Unknown key behavior is governed by BindJSONOptions.ErrorUnused.
//
// Examples:
//
//	type User struct {
//		ID   int    `json:"id"`
//		Name string `json:"name"`
//	}
//	m := map[string]any{"id": "10", "name": "Ada"}
//	var u User
//	// Coerce string to int
//	_ = c.BindMap(&u, m, BindJSONOptions{WeaklyTypedInput: true})
//
//	// Strict mode rejects unknown fields
//	m2 := map[string]any{"id": 1, "name": "Ada", "extra": true}
//	if err := c.BindMap(&u, m2, BindJSONOptions{ErrorUnused: true}); err != nil {
//		// err can be converted to FieldErrors indicating "extra" is unexpected
//	}
func (c *DefaultContext) BindMap(v any, m map[string]any, opts ...BindJSONOptions) error {
	var o BindJSONOptions
	if len(opts) > 0 {
		o = opts[0]
	} else {
		o.ErrorUnused = true
	}

	// Target struct type for better error messages
	var targetType reflect.Type
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Ptr && !rv.IsNil() && rv.Elem().Kind() == reflect.Struct {
		targetType = rv.Elem().Type()
	}

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
		if fe := mapMapStructureError(err, o, targetType); fe != nil {
			return fe
		}
		return err
	}
	return nil
}

// BindForm collects form body fields and binds them into v.
// Supports application/x-www-form-urlencoded and multipart/form-data (textual fields only).
//
// For multipart/form-data, file uploads are ignored here; only textual values are bound.
//
// Examples:
//
//	// Content-Type: application/x-www-form-urlencoded
//	// Body: name=Ada&age=33
//	type Form struct { Name string `json:"name"`; Age int `json:"age"` }
//	var f Form
//	_ = c.BindForm(&f)
//
//	// Multipart: text fields collected from r.MultipartForm.Value
//	_ = c.BindForm(&f)
func (c *DefaultContext) BindForm(v any, opts ...BindJSONOptions) error {
	m, err := c.collectFormMap()
	if err != nil {
		return err
	}
	return c.BindMap(v, m, opts...)
}

// BindQuery collects query string parameters and binds them into v.
// Only the first value per key is used, matching typical form semantics.
//
// Example:
//
//	// GET /search?q=flash&page=2
//	type Q struct { Q string `json:"q"`; Page int `json:"page"` }
//	var q Q
//	_ = c.BindQuery(&q)
func (c *DefaultContext) BindQuery(v any, opts ...BindJSONOptions) error {
	return c.BindMap(v, c.collectQueryMap(), opts...)
}

// BindPath collects path parameters and binds them into v.
// The keys correspond to route parameter names (e.g., ":id").
//
// Example:
//
//	// Route: /users/:id
//	type P struct { ID int `json:"id"` }
//	var p P
//	_ = c.BindPath(&p)
func (c *DefaultContext) BindPath(v any, opts ...BindJSONOptions) error {
	return c.BindMap(v, c.collectPathMap(), opts...)
}

// BindAny merges values from query, body (Form then JSON), and path, and binds them into v.
// Precedence (highest wins): Path > Body > Query, and within Body: JSON > Form.
//
// This is convenient for handlers that accept input from multiple sources while
// maintaining a single struct definition.
//
// Examples:
//
//	// Route: /users/:id
//	// Request: GET /users/10?active=true
//	// Body: {"name":"Ada"}
//	type In struct {
//		ID     int    `json:"id"`
//		Name   string `json:"name"`
//		Active bool   `json:"active"`
//	}
//	var in In
//	_ = c.BindAny(&in) // => ID=10 (path), Name="Ada" (json), Active=true (query)
//
//	// Form vs JSON precedence: JSON overrides Form for keys present in both
//	// Body: name="A" (form) and {"name":"B"} (json) => name becomes "B"
func (c *DefaultContext) BindAny(v any, opts ...BindJSONOptions) error {
	// Pre-size map to reduce growth rehashing
	est := len(c.r.URL.Query()) + len(c.params)
	if c.r.PostForm != nil {
		est += len(c.r.PostForm)
	}
	if c.r.MultipartForm != nil && c.r.MultipartForm.Value != nil {
		est += len(c.r.MultipartForm.Value)
	}
	out := make(map[string]any, est)

	// Lowest priority first: Query
	c.collectQueryInto(out)

	// Body: Form then JSON (JSON overrides Form)
	ct := c.r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType == "application/x-www-form-urlencoded" || strings.HasPrefix(mediaType, "multipart/") {
		if err := c.collectFormInto(out); err != nil {
			return err
		}
	}
	if strings.Contains(mediaType, "+json") || mediaType == "application/json" {
		jm, err := c.collectJSONMap()
		if err != nil {
			return err
		}
		mergeInto(out, jm, false)
	}

	// Highest: Path
	c.collectPathInto(out)

	return c.BindMap(v, out, opts...)
}

// collectJSONMap reads body and parses into map[string]any. Honors default strictness at BindMap stage.
func (c *DefaultContext) collectJSONMap() (map[string]any, error) {
	defer c.r.Body.Close()
	var m map[string]any
	dec := json.NewDecoder(c.r.Body)
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// collectFormMap parses the request form and returns a map[string]any using first value per key.
func (c *DefaultContext) collectFormMap() (map[string]any, error) {
	// ParseForm handles both x-www-form-urlencoded and multipart/form-data
	if err := c.r.ParseForm(); err != nil {
		return nil, err
	}
	// For multipart/form-data, ensure MultipartForm is populated
	if ct := c.r.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/") && c.r.MultipartForm == nil {
		// Use a reasonable default memory limit similar to net/http server
		if err := c.r.ParseMultipartForm(32 << 20); err != nil { // 32 MB
			return nil, err
		}
	}
	// Prefer PostForm values; also include multipart textual values
	out := valuesToMap(c.r.PostForm)
	if c.r.MultipartForm != nil && c.r.MultipartForm.Value != nil {
		for k, vals := range c.r.MultipartForm.Value {
			if len(vals) > 0 {
				// If key already present from PostForm, keep existing (PostForm first)
				if _, ok := out[k]; !ok {
					out[k] = vals[0]
				}
			}
		}
	}
	return out, nil
}

// collectQueryMap returns a map from URL query parameters (first value per key).
func (c *DefaultContext) collectQueryMap() map[string]any {
	return valuesToMap(c.r.URL.Query())
}

// collectQueryInto writes first query values into dst (no intermediate map).
func (c *DefaultContext) collectQueryInto(dst map[string]any) {
	for k, vals := range c.r.URL.Query() {
		if len(vals) > 0 {
			dst[k] = vals[0]
		}
	}
}

// collectPathMap returns a map from route params.
func (c *DefaultContext) collectPathMap() map[string]any {
	out := map[string]any{}
	for _, p := range c.params {
		out[p.Key] = p.Value
	}
	return out
}

// collectPathInto writes path params into dst (no intermediate map).
func (c *DefaultContext) collectPathInto(dst map[string]any) {
	for _, p := range c.params {
		dst[p.Key] = p.Value
	}
}

// collectFormInto parses the form and writes first values into dst (no intermediate map).
func (c *DefaultContext) collectFormInto(dst map[string]any) error {
	if err := c.r.ParseForm(); err != nil {
		return err
	}
	if ct := c.r.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/") && c.r.MultipartForm == nil {
		if err := c.r.ParseMultipartForm(32 << 20); err != nil { // 32 MB
			return err
		}
	}
	for k, vals := range c.r.PostForm {
		if len(vals) > 0 {
			dst[k] = vals[0]
		}
	}
	if c.r.MultipartForm != nil && c.r.MultipartForm.Value != nil {
		for k, vals := range c.r.MultipartForm.Value {
			if len(vals) > 0 {
				dst[k] = vals[0]
			}
		}
	}
	return nil
}

// valuesToMap converts url.Values into map[string]any taking the first value for each key.
func valuesToMap(v url.Values) map[string]any {
	out := map[string]any{}
	for k, vals := range v {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

// mergeInto copies all keys from src into dst. If preserve is true, existing keys in dst are kept.
func mergeInto(dst, src map[string]any, preserve bool) {
	for k, v := range src {
		if preserve {
			if _, ok := dst[k]; ok {
				continue
			}
		}
		dst[k] = v
	}
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

// mapMapStructureError converts map structure errors into FieldErrors with friendly messages.
func mapMapStructureError(err error, o BindJSONOptions, targetType reflect.Type) error {
	// Map structure may return a multi-error string; handle key cases.
	s := err.Error()
	// Unknown field when ErrorUnused is true: "has invalid keys: asdf, ..."
	if o.ErrorUnused {
		if strings.Contains(s, "has invalid keys:") {
			// Extract only the comma-separated keys on the same line as the marker.
			marker := "has invalid keys:"
			idx := strings.Index(s, marker)
			if idx != -1 {
				list := s[idx+len(marker):]
				// Cut at first newline to avoid pulling in subsequent error bullets.
				if nl := strings.IndexByte(list, '\n'); nl != -1 {
					list = list[:nl]
				}
				// Normalize whitespace and bullets
				list = strings.TrimSpace(list)
				// Split by comma and trim punctuation/whitespace around keys
				parts := strings.Split(list, ",")
				fe := map[string]string{}
				for _, p := range parts {
					k := strings.TrimSpace(p)
					// remove any leading bullet or quotes
					k = strings.TrimLeft(k, "* '`\"")
					// strip trailing punctuation if present
					k = strings.Trim(k, "'`\" .;:")
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
	// Type mismatch when WeaklyTypedInput is false. map structure reports e.g.:
	// "cannot decode 'age' from string into int"
	if !o.WeaklyTypedInput {
		if field, ok := extractFieldFromMapStructureTypeError(s); ok {
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

// extractFieldFromMapStructureTypeError extracts the field name from a map structure type error string.
func extractFieldFromMapStructureTypeError(s string) (string, bool) {
	if strings.HasPrefix(s, " error(s) decoding:") {
		lines := strings.Split(s, "\n")
		// pick the last non-empty line (map structure formats each error as a bullet line)
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				s = line
				break
			}
		}
	}
	start := strings.Index(s, "cannot decode '")
	if start == -1 {
		start = strings.Index(s, "invalid type for '")
		if start == -1 {
			// Newer map structure error style: "* '<field>' expected type 'string', got ..."
			// Find the first quoted token as the field name, followed by " expected type '"
			// Strip any bullet prefix like "* ".
			s2 := strings.TrimSpace(strings.TrimPrefix(s, "* "))
			q1 := strings.IndexByte(s2, '\'')
			if q1 == -1 {
				return "", false
			}
			q2 := strings.IndexByte(s2[q1+1:], '\'')
			if q2 == -1 {
				return "", false
			}
			field := s2[q1+1 : q1+1+q2]
			// sanity check that this indeed is the expected type pattern
			if strings.Contains(s2[q1+1+q2+1:], " expected type '") {
				return field, true
			}
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
