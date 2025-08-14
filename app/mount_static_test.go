package app

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type fakeFS struct{ err error }

func (f fakeFS) Open(name string) (http.File, error) { return nil, f.err }

func TestStaticDirs_NoDirs_NoRoutes(t *testing.T) {
	a := New()
	// No directories passed -> should not register any static routes
	a.StaticDirs("/assets")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/any.txt", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound { // no handler registered
		t.Fatalf("expected 404 when no dirs, got %d", rec.Code)
	}
}

func TestStaticDirs_PrefixNormalization_And_EmptyDirIgnored(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	// Provide prefix without leading/trailing slash and include an empty dir entry
	a.StaticDirs("assets", "", dir)

	// GET should serve
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/assets/hello.txt", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
			t.Fatalf("GET static failed: %d %q", rec.Code, rec.Body.String())
		}
	}
	// HEAD should also be wired and return no body
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodHead, "/assets/hello.txt", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("HEAD static failed: %d", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("HEAD should not return a body, got %d bytes", rec.Body.Len())
		}
	}
}

func TestStaticDirs_MultiDir_FirstMatchWinsAndFallback(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	// a.txt in both dirs with different contents -> first dir should win
	if err := os.WriteFile(filepath.Join(d1, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d2, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	// b.txt only in second dir -> should fallback
	if err := os.WriteFile(filepath.Join(d2, "b.txt"), []byte("bee"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.StaticDirs("/s", d1, d2)

	// First match wins
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/s/a.txt", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "one" {
			t.Fatalf("expected first dir content, got %d %q", rec.Code, rec.Body.String())
		}
	}
	// Fallback to second dir
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/s/b.txt", nil)
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "bee" {
			t.Fatalf("expected second dir content, got %d %q", rec.Code, rec.Body.String())
		}
	}
}

func TestStaticDirs_PrefixAlreadyHasTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello2.txt")
	if err := os.WriteFile(p, []byte("hiya"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	// prefix already ends with '/'
	a.StaticDirs("/pref/", dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pref/hello2.txt", nil)
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hiya" {
		t.Fatalf("static (trailing slash) failed: %d %q", rec.Code, rec.Body.String())
	}
}

func TestMultiFS_Open_Behavior(t *testing.T) {
	// 1) Empty multiFS -> os.ErrNotExist
	{
		var m multiFS
		f, err := m.Open("x")
		if f != nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("empty multiFS should return os.ErrNotExist, got %v %v", f, err)
		}
	}
	// 2) First not-exist, then success
	{
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "ok.txt"), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := multiFS{fakeFS{err: fs.ErrNotExist}, http.Dir(d)}
		f, err := m.Open("ok.txt")
		if err != nil || f == nil {
			t.Fatalf("expected success, got f=%v err=%v", f, err)
		}
		_ = f.Close()
	}
	// 3) Non-NotExist error should be returned if all fail (lastErr preserved)
	{
		errPerm := fs.ErrPermission
		m := multiFS{fakeFS{err: fs.ErrNotExist}, fakeFS{err: errPerm}}
		f, err := m.Open("nope.txt")
		if f != nil || !errors.Is(err, errPerm) {
			t.Fatalf("expected permission error, got f=%v err=%v", f, err)
		}
	}
	// 4) First returns a non-NotExist error, second returns NotExist -> lastErr should be NotExist
	{
		errPerm := fs.ErrPermission
		m := multiFS{fakeFS{err: errPerm}, fakeFS{err: fs.ErrNotExist}}
		f, err := m.Open("nope-again.txt")
		if f != nil || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected not-exist error, got f=%v err=%v", f, err)
		}
	}
}
