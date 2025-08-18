package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goflash/flash/v1"
)

func TestRecoverMiddleware(t *testing.T) {
	a := flash.New()
	a.Use(Recover())
	a.GET("/panic", func(c *flash.Ctx) error { panic("boom") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
