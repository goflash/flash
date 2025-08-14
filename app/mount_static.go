package app

import (
	"net/http"
	"os"
	"strings"
)

// HandleHTTP mounts a net/http.Handler on a specific HTTP method and path.
// This allows interoperability with standard library handlers.
func (a *App) HandleHTTP(method, path string, h http.Handler) { a.router.Handler(method, path, h) }

// Mount mounts a net/http.Handler for all common HTTP methods (GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD)
// under the given path. Useful for mounting sub-routers or third-party handlers.
func (a *App) Mount(path string, h http.Handler) {
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead} {
		a.router.Handler(m, path, h)
	}
}

// Static serves files from one or more directories under a URL prefix for GET and HEAD requests.
// If multiple directories are provided (via StaticDirs), files are resolved in order, first match wins.
func (a *App) Static(prefix, dir string) { a.StaticDirs(prefix, dir) }

// StaticDirs serves files from multiple directories under the same URL prefix (GET and HEAD).
// Directories are searched in order; the first existing file is served.
// This mirrors frameworks like Fiber where multiple folders can back the same route.
func (a *App) StaticDirs(prefix string, dirs ...string) {
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

// multiFS is an http.FileSystem that tries multiple underlying filesystems in order.
// The first successful Open wins; if all fail with not-exist, returns os.ErrNotExist.
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
