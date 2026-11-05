package spa_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"brevis.sh/brevis/web/spa"
)

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	spa.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

// THE SHELL ANSWERS THE APP'S ROUTES: the root and any client-side route
// (which the server has never heard of) are index.html, revalidated by digest.
func TestRoutesAreTheShell(t *testing.T) {
	for _, target := range []string{"/", "/runs/0d0e88ab", "/login", "/ingest/streams/gw/v1/a.b"} {
		w := get(t, target)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="root"`) {
			t.Errorf("%s: %d, want the shell", target, w.Code)
		}
		if w.Header().Get("ETag") == "" || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: the shell needs a validator and no-cache, got %q / %q", target, w.Header().Get("ETag"), w.Header().Get("Cache-Control"))
		}
	}
}

// EVERYTHING THE SHELL ASKS FOR IS SERVED, and cached for good: it is named by
// its hash. The same walk assets_test does for the server-rendered pages.
func TestWhatTheShellReferencesIsServed(t *testing.T) {
	refs := regexp.MustCompile(`(?:src|href)="(/static/[^"]+)"`).FindAllStringSubmatch(get(t, "/").Body.String(), -1)
	if len(refs) == 0 {
		t.Fatal("the shell references no asset: is dist/ built?")
	}
	for _, m := range refs {
		w := get(t, m[1])
		if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
			t.Errorf("%s: %d, cache %q; want 200 and immutable", m[1], w.Code, w.Header().Get("Cache-Control"))
		}
	}
}

// A MISSING FILE IS A 404, NOT THE SHELL: a stale hashed asset answered with
// HTML would be a script that fails to parse.
func TestAMissingFileIsNotTheShell(t *testing.T) {
	if w := get(t, "/static/index-gone.js"); w.Code != http.StatusNotFound {
		t.Errorf("a missing asset: %d, want 404", w.Code)
	}
}

// THE SHELL WEARS THE CONSOLE'S OWN DESIGN: it links the server's stylesheet by
// its versioned URL (not the placeholder), so tokens, fonts and the brand's theme
// are the ones the server pages use.
func TestTheShellLinksTheConsolesStylesheet(t *testing.T) {
	body := get(t, "/").Body.String()
	if strings.Contains(body, "brevis:app-css") {
		t.Error("the stylesheet placeholder was not replaced")
	}
	if !regexp.MustCompile(`href="/assets/app\.css\?v=[0-9a-f]{12}"`).MatchString(body) {
		t.Errorf("the shell does not link /assets/app.css at its current version:\n%s", body)
	}
}

// AN UNKNOWN /api PATH IS JSON, NOT A PAGE: a client reading data must not be handed HTML.
func TestAnUnknownAPIPathIsAJSON404(t *testing.T) {
	w := get(t, "/api/nope")
	if w.Code != http.StatusNotFound || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") ||
		strings.TrimSpace(w.Body.String()) != `{"error":"not_found"}` {
		t.Errorf("/api/nope: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

// THE OLD /app/ ADDRESSES REDIRECT, path and query kept, and never off-site.
func TestLegacyAppPathsRedirectToTheRoot(t *testing.T) {
	for from, want := range map[string]string{
		"/app": "/", "/app/": "/", "/app/runs": "/runs", "/app/runs?state=failed&page=2": "/runs?state=failed&page=2",
		"/app/ingest/loads/orders/load_0": "/ingest/loads/orders/load_0", "/app/%5Cevil.com": "/",
	} {
		w := httptest.NewRecorder()
		spa.LegacyRedirect(w, httptest.NewRequest(http.MethodGet, from, nil))
		if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != want {
			t.Errorf("%s: %d -> %q, want 308 -> %q", from, w.Code, w.Header().Get("Location"), want)
		}
	}
}
