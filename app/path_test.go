package app

import "testing"

func TestCleanPath(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		"users":         "/users",
		"/users/..//a/": "/a",
	}
	for in, want := range cases {
		if got := cleanPath(in); got != want {
			t.Fatalf("cleanPath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestJoinPath(t *testing.T) {
	if got := joinPath("/api", "/v1"); got != "/api/v1" {
		t.Fatalf("joinPath got %q", got)
	}
	if got := joinPath("/", "/v1"); got != "/v1" {
		t.Fatalf("joinPath got %q", got)
	}
	if got := joinPath("/api", "/"); got != "/api" {
		t.Fatalf("joinPath got %q", got)
	}
}
