package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppMethodHelpersV2(t *testing.T) {
	a := New()
	a.POST("/p", func(c *Ctx) error { return c.String(http.StatusOK, "P") })
	a.PUT("/u", func(c *Ctx) error { return c.String(http.StatusOK, "U") })
	a.PATCH("/pa", func(c *Ctx) error { return c.String(http.StatusOK, "PA") })
	a.DELETE("/d", func(c *Ctx) error { return c.String(http.StatusOK, "D") })
	a.OPTIONS("/o", func(c *Ctx) error { return c.String(http.StatusOK, "O") })
	a.HEAD("/h", func(c *Ctx) error { return c.String(http.StatusOK, "") })

	tests := []struct{ m, p, want string }{
		{http.MethodPost, "/p", "P"},
		{http.MethodPut, "/u", "U"},
		{http.MethodPatch, "/pa", "PA"},
		{http.MethodDelete, "/d", "D"},
		{http.MethodOptions, "/o", "O"},
		{http.MethodHead, "/h", ""},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tt.m, tt.p, nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != tt.want {
			t.Fatalf("%s %s -> %d %q", tt.m, tt.p, rec.Code, rec.Body.String())
		}
	}
}

func TestRouterAllMethodsHelpersV2(t *testing.T) {
	a := New()
	g := a.Group("/r")
	g.GET("/get", func(c *Ctx) error { return c.String(http.StatusOK, "GET") })
	g.POST("/post", func(c *Ctx) error { return c.String(http.StatusOK, "POST") })
	g.PUT("/put", func(c *Ctx) error { return c.String(http.StatusOK, "PUT") })
	g.PATCH("/patch", func(c *Ctx) error { return c.String(http.StatusOK, "PATCH") })
	g.DELETE("/delete", func(c *Ctx) error { return c.String(http.StatusOK, "DELETE") })
	g.OPTIONS("/options", func(c *Ctx) error { return c.String(http.StatusOK, "OPTIONS") })
	g.HEAD("/head", func(c *Ctx) error { return c.String(http.StatusOK, "") })

	tests := []struct{ method, path, want string }{
		{http.MethodPost, "/r/post", "POST"},
		{http.MethodPut, "/r/put", "PUT"},
		{http.MethodPatch, "/r/patch", "PATCH"},
		{http.MethodDelete, "/r/delete", "DELETE"},
		{http.MethodOptions, "/r/options", "OPTIONS"},
		{http.MethodHead, "/r/head", ""},
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
