package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	validator "github.com/go-playground/validator/v10"
	"github.com/goflash/flash/v1"
	"github.com/goflash/flash/v1/validate"
)

// simple handler that triggers validation and returns mapped field errors
func validateHandler(c *flash.Ctx) error {
	type U struct {
		Name string `json:"name" validate:"required"`
	}
	var u U
	// No body -> required fails
	if err := validate.Struct(u); err != nil {
		m := validate.ToFieldErrorsWithContext(c.Context(), err)
		return c.JSON(m)
	}
	return c.JSON(map[string]any{"ok": true})
}

func TestValidatorI18n_DefaultLocaleAndLangParam(t *testing.T) {
	defer validate.SetMessageFunc(nil)
	app := flash.New()

	// Configure MessageFuncFor with simple strings to avoid bringing i18n deps into tests
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale: "en",
		MessageFuncFor: func(locale string) func(validator.FieldError) string {
			if locale == "es" {
				return func(validator.FieldError) string { return "ES_MSG" }
			}
			return func(validator.FieldError) string { return "EN_MSG" }
		},
		SetGlobal: true,
	}))

	app.POST("/:lang/test", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/es/test", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || body == "{}" || body == "{\n}\n" {
		t.Fatalf("expected JSON body with field message, got %q", body)
	}
	if want := "ES_MSG"; body == want || !contains(body, want) {
		t.Fatalf("expected spanish message %q in body, got %q", want, body)
	}
}

func TestValidatorI18n_DefaultFallback(t *testing.T) {
	defer validate.SetMessageFunc(nil)
	app := flash.New()
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale: "en",
		MessageFuncFor: func(locale string) func(validator.FieldError) string {
			return func(validator.FieldError) string { return "MSG_" + locale }
		},
		SetGlobal: true,
	}))
	app.POST("/test", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "MSG_en") {
		t.Fatalf("expected EN fallback, got %q", body)
	}
}

func TestValidatorI18n_NoMessageFuncConfigured_NoOp(t *testing.T) {
	validate.SetMessageFunc(nil)
	app := flash.New()
	// No MessageFuncFor provided: middleware should be a no-op and not crash
	app.Use(ValidatorI18n(ValidatorI18nConfig{}))
	app.POST("/noop", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/noop", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Should fall back to defaultMessage (English-like)
	if body := rec.Body.String(); !contains(body, "is required") {
		t.Fatalf("expected default message in body, got %q", body)
	}
}

func TestValidatorI18n_UsesLocaleFromCtx(t *testing.T) {
	validate.SetMessageFunc(nil)
	app := flash.New()
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale: "en",
		LocaleFromCtx: func(c *flash.Ctx) string { return "es" },
		MessageFuncFor: func(locale string) func(validator.FieldError) string {
			if locale == "es" {
				return func(validator.FieldError) string { return "ES_FROM_CTX" }
			}
			return func(validator.FieldError) string { return "EN_DEFAULT" }
		},
	}))
	app.POST("/ctx", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/ctx", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "ES_FROM_CTX") {
		t.Fatalf("expected ES_FROM_CTX, got %q", body)
	}
}

func TestValidatorI18n_FallbackToDefaultWhenNilForLocale(t *testing.T) {
	validate.SetMessageFunc(nil)
	app := flash.New()
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale: "en",
		MessageFuncFor: func(locale string) func(validator.FieldError) string {
			if locale == "en" {
				return func(validator.FieldError) string { return "EN_FALLBACK" }
			}
			return nil // force fallback path
		},
	}))
	app.POST("/:lang/test", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/fr/test", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "EN_FALLBACK") {
		t.Fatalf("expected EN_FALLBACK, got %q", body)
	}
}

func TestValidatorI18n_NoFuncEvenForDefault_SkipsSetting(t *testing.T) {
	validate.SetMessageFunc(nil)
	app := flash.New()
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale:  "en",
		MessageFuncFor: func(locale string) func(validator.FieldError) string { return nil },
	}))
	app.POST("/def", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/def", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Falls back to defaultMessage
	if body := rec.Body.String(); !contains(body, "is required") {
		t.Fatalf("expected default message, got %q", body)
	}
}

func TestValidatorI18n_EmptyDefaultLocaleBecomesEN(t *testing.T) {
	validate.SetMessageFunc(nil)
	app := flash.New()
	app.Use(ValidatorI18n(ValidatorI18nConfig{
		DefaultLocale: "",
		MessageFuncFor: func(locale string) func(validator.FieldError) string {
			return func(validator.FieldError) string { return "MSG_" + locale }
		},
	}))
	app.POST("/empty", validateHandler)

	req := httptest.NewRequest(http.MethodPost, "/empty", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "MSG_en") {
		t.Fatalf("expected MSG_en due to empty default -> en, got %q", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
