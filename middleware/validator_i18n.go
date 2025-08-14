package middleware

import (
	"strings"

	validator "github.com/go-playground/validator/v10"
	"github.com/goflash/flash"
	"github.com/goflash/flash/validate"
)

// ValidatorI18nConfig configures the ValidatorI18n middleware.
// The application supplies how to get a message function for a given locale.
// No locale or translation packages are imported by the framework.
type ValidatorI18nConfig struct {
	// DefaultLocale used when none is derived from the request.
	DefaultLocale string
	// LocaleFromCtx returns the desired locale for a request.
	// Default: use lowercased route param ":lang" if present.
	LocaleFromCtx func(c *flash.Ctx) string
	// MessageFuncFor returns a function that translates a FieldError into a message
	// for a given locale. The application typically closes over its prepared
	// translators and returns: func(fe validator.FieldError) string { return fe.Translate(trans) }.
	// Required.
	MessageFuncFor func(locale string) func(validator.FieldError) string
	// SetGlobal optionally sets the global fallback message function to DefaultLocale,
	// used when no per-request function was provided.
	SetGlobal bool
}

// ValidatorI18n returns middleware that attaches a request-scoped validator message
// function to the request context based on a locale.
func ValidatorI18n(cfg ValidatorI18nConfig) flash.Middleware {
	if cfg.MessageFuncFor == nil {
		// No-op middleware if misconfigured
		return func(next flash.Handler) flash.Handler { return next }
	}
	if cfg.DefaultLocale == "" {
		cfg.DefaultLocale = "en"
	}
	if cfg.SetGlobal {
		if mf := cfg.MessageFuncFor(cfg.DefaultLocale); mf != nil {
			validate.SetMessageFunc(mf)
		}
	}

	return func(next flash.Handler) flash.Handler {
		return func(c *flash.Ctx) error {
			locale := cfg.DefaultLocale
			if cfg.LocaleFromCtx != nil {
				if l := cfg.LocaleFromCtx(c); l != "" {
					locale = strings.ToLower(l)
				}
			} else {
				if l := c.Param("lang"); l != "" {
					locale = strings.ToLower(l)
				}
			}

			mf := cfg.MessageFuncFor(locale)
			if mf == nil && locale != cfg.DefaultLocale {
				mf = cfg.MessageFuncFor(cfg.DefaultLocale)
			}
			if mf != nil {
				ctx := validate.WithMessageFunc(c.Context(), mf)
				// propagate context to request
				r2 := c.Request().WithContext(ctx)
				c.SetRequest(r2)
			}
			return next(c)
		}
	}
}
