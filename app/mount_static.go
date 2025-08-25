package app

import (
	"net/http"
	"os"
	"strings"
)

// HandleHTTP mounts a net/http.Handler on a specific HTTP method and path.
// This enables interoperability with standard library handlers or third-party
// http.Handler implementations without adapting to app.Handler.
//
// The handler receives the raw http.ResponseWriter and *http.Request. Use this
// method when you want to pass through to an existing handler as-is.
//
// Example:
//
//	a := app.New()
//	a.HandleHTTP(http.MethodGet, "/metrics", promhttp.Handler())
//	_ = http.ListenAndServe(":8080", a)
func (a *DefaultApp) HandleHTTP(method, path string, h http.Handler) {
	a.router.Handler(method, path, h)
}

// Mount mounts a net/http.Handler for all common HTTP methods (GET, POST, PUT,
// PATCH, DELETE, OPTIONS, HEAD) under the given path.
//
// This is useful for mounting sub-routers or third-party handlers that already
// implement routing internally (e.g., a GraphQL handler, an admin console).
//
// Example (mounting a sub-router):
//
//	sr := http.NewServeMux()
//	sr.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
//	a := app.New()
//	a.Mount("/admin", sr)
//	// Now /admin/health is served by sr for GET/POST/PUT/PATCH/DELETE/OPTIONS/HEAD
func (a *DefaultApp) Mount(path string, h http.Handler) {
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead} {
		a.router.Handler(m, path, h)
	}
}

// Static serves files from a directory under a URL prefix for GET and HEAD
// requests. This is a convenience that delegates to StaticDirs with a single
// directory.
//
// Example:
//
//	a := app.New()
//	a.Static("/assets", "./public")
//	// Serves GET/HEAD /assets/* from files under ./public
func (a *DefaultApp) Static(prefix, dir string) { a.StaticDirs(prefix, dir) }

// StaticDirs serves files from multiple directories under the same URL prefix
// for GET and HEAD requests. Directories are searched in order; the first
// existing file is served. This mirrors frameworks like Fiber where multiple
// folders can back the same route.
//
// The prefix is normalized and must end with a trailing slash; a catch-all
// pattern "*filepath" is registered under that prefix. The underlying handler
// strips the prefix before serving from the merged filesystem.
//
// Security considerations:
//   - Ensure you only expose directories meant to be public
//   - Avoid serving dotfiles if not intended (http.FileServer will serve them)
//   - Consider setting appropriate Cache-Control headers via middleware
//
// Examples:
//
//	// Single directory equivalent to Static("/assets", "./public")
//	a.StaticDirs("/assets", "./public")
//
//	// Multiple directories: look in ./public first, then ./themes/default
//	a.StaticDirs("/assets", "./public", "./themes/default")
//	// /assets/logo.png will be served from the first directory that contains it
func (a *DefaultApp) StaticDirs(prefix string, dirs ...string) {
	prefix = cleanPath(prefix)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	// Build a multi-filesystem from the provided directories
	mfs := multiFS{}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		mfs = append(mfs, http.Dir(d))
	}
	// If no valid dirs, do nothing
	if len(mfs) == 0 {
		return
	}

	fs := http.FileServer(mfs)
	h := http.StripPrefix(prefix, fs)
	a.router.Handler(http.MethodGet, prefix+"*filepath", h)
	a.router.Handler(http.MethodHead, prefix+"*filepath", h)
}

// multiFS is an http.FileSystem that tries multiple underlying filesystems in
// order. The first successful Open wins; if all fail with os.ErrNotExist,
// multiFS returns os.ErrNotExist.
//
// This allows StaticDirs to overlay multiple directories, similar to how web
// servers can serve from a chain of roots.
//
// Example:
//
//	mfs := multiFS{http.Dir("./public"), http.Dir("./theme")}
//	_, err := mfs.Open("/logo.png")
//	_ = err
type multiFS []http.FileSystem

func (m multiFS) Open(name string) (http.File, error) {
	var lastErr error
	for _, fs := range m {
		f, err := fs.Open(name)
		if err == nil {
			return f, nil
		}
		// Prefer to continue on not-exist; remember last error for context
		lastErr = err
		if os.IsNotExist(err) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastErr
}
