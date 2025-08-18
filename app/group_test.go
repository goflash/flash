package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroupAndNestedMiddleware(t *testing.T) {
	a := New()
	g := a.Group("/api", func(next Handler) Handler { return func(c Ctx) error { c.Header("X-API", "1"); return next(c) } })
	v1 := g.Group("/v1", func(next Handler) Handler { return func(c Ctx) error { c.Header("X-V", "1"); return next(c) } })
	v1.GET("/ping", func(c Ctx) error { return c.String(http.StatusOK, "pong") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("X-API") != "1" || rec.Header().Get("X-V") != "1" {
		t.Fatalf("missing headers")
	}
}

func TestGroupUseAddsMiddleware(t *testing.T) {
	a := New()
	g := a.Group("/api")
	g.Use(func(next Handler) Handler { return func(c Ctx) error { c.Header("X-GU", "1"); return next(c) } })
	g.GET("/x", func(c Ctx) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	a.ServeHTTP(rec, req)
	if rec.Header().Get("X-GU") != "1" {
		t.Fatalf("group Use middleware not applied")
	}
}

func TestGroupMethodHelpersCoverage(t *testing.T) {
	a := New()
	g := a.Group("/g")
	g.POST("/post", func(c Ctx) error { return c.String(http.StatusOK, "POST") })
	g.PUT("/put", func(c Ctx) error { return c.String(http.StatusOK, "PUT") })
	g.PATCH("/patch", func(c Ctx) error { return c.String(http.StatusOK, "PATCH") })
	g.DELETE("/delete", func(c Ctx) error { return c.String(http.StatusOK, "DELETE") })
	g.OPTIONS("/options", func(c Ctx) error { return c.String(http.StatusOK, "OPTIONS") })
	g.HEAD("/head", func(c Ctx) error { return c.String(http.StatusOK, "") })

	tests := []struct{ method, path, want string }{
		{http.MethodPost, "/g/post", "POST"},
		{http.MethodPut, "/g/put", "PUT"},
		{http.MethodPatch, "/g/patch", "PATCH"},
		{http.MethodDelete, "/g/delete", "DELETE"},
		{http.MethodOptions, "/g/options", "OPTIONS"},
		{http.MethodHead, "/g/head", ""},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s -> %d", tt.method, tt.path, rec.Code)
		}
		if rec.Body.String() != tt.want {
			t.Fatalf("body=%q want %q", rec.Body.String(), tt.want)
		}
	}
}
