// Package spa serves the React app (web/spa/src) from the binary.
//
// dist/ is the Vite build, committed like web/assets/app.css: the image needs
// no Node, and //go:embed fails the Go build if dist/ is missing, not
// production with a blank page. `make generate` rebuilds it.
package spa

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/web/assets"
)

//go:embed dist
var dist embed.FS

// Assets is where the build's hashed files live (vite.config.ts `assetsDir`).
// It is not "/assets/": that one is the engine's own (app.css, fonts, logo).
const Assets = "/static/"

// Handler serves the console: the shell for every page and dist/static/ for
// its files.
//
// The server knows no client route, so any GET that no other handler claimed
// is a page and gets index.html (history fallback). Two exceptions: a path
// under Assets is a file or a 404 -- answering a missing /static/x.js with the
// HTML shell would turn a stale asset into a script that fails to parse, which
// reads as a bug in the page and not in the cache -- and an unknown /api/ path
// is a JSON 404, not a page.
//
// Vite names everything under static/ by its content hash, so those are cached
// for a year; index.html is the one file whose name never changes and is
// revalidated by digest.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	// The shell carries a placeholder where the console's own stylesheet goes
	// (index.html). It is replaced once, here, by the versioned URL, cached for
	// a year until the sheet changes. Vite's dev server does the same swap with
	// a plugin (vite.config.ts).
	shell, _ := fs.ReadFile(root, "index.html")
	shell = bytes.Replace(shell, []byte("<!--brevis:app-css-->"),
		[]byte(`<link rel="stylesheet" href="`+assets.URL("app.css")+`">`), 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}` + "\n"))
			return
		}
		if strings.HasPrefix(r.URL.Path, Assets) {
			name := strings.TrimPrefix(r.URL.Path, "/")
			b, err := fs.ReadFile(root, name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(b))
			return
		}
		b := shell
		if len(b) == 0 {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(b)
		w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
	})
}

// LegacyRedirect sends /app and /app/... -- where the console lived while it
// was being built -- to the same page at the root, query kept, so bookmarks work.
func LegacyRedirect(w http.ResponseWriter, r *http.Request) {
	to := "/" + strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/app"), "/")
	if r.URL.RawQuery != "" {
		to += "?" + r.URL.RawQuery
	}
	// auth.Target refuses `//host` and a backslash (a browser reads both as another origin).
	http.Redirect(w, r, auth.Target(to), http.StatusPermanentRedirect)
}
