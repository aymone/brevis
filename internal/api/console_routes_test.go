package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/auth"
)

func consoleServer(t *testing.T, cred auth.Credential) *api.Server {
	t.Helper()
	return api.NewServerWithAuth(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, dataUI(catalogFake{}), cred, true)
}

func serve(s *api.Server, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// THE CONSOLE LIVES AT THE ROOT: any GET no engine route claims is a client
// route and gets the shell, and the engine's own paths still win over it.
func TestTheConsoleIsServedAtTheRootWithAHistoryFallback(t *testing.T) {
	s := consoleServer(t, auth.Credential{})
	for _, p := range []string{"/", "/runs", "/runs/0d0e88ab", "/ingest/streams/gw/v1/a.b", "/data/target?u=bigquery://p.d.t", "/login", "/nope"} {
		if rec := serve(s, "GET", p); rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("GET %s = %d, want the shell", p, rec.Code)
		}
	}
	if rec := serve(s, "GET", "/assets/app.css"); rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Errorf("/assets/app.css = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := serve(s, "GET", "/api/palette"); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("/api/palette = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := serve(s, "GET", "/health"); rec.Code != 200 || strings.Contains(rec.Body.String(), `id="root"`) {
		t.Errorf("/health = %d: the shell must not hide it", rec.Code)
	}
}

func TestAnUnknownAPIPathIsAJSON404NotAPage(t *testing.T) {
	rec := serve(consoleServer(t, auth.Credential{}), "GET", "/api/nope")
	if rec.Code != 404 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("/api/nope = %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

func TestTheOldAppAddressesRedirectToTheRoot(t *testing.T) {
	s := consoleServer(t, auth.Credential{})
	for from, want := range map[string]string{"/app": "/", "/app/": "/", "/app/runs?state=failed": "/runs?state=failed", "/app/ingest/loads/a/b": "/ingest/loads/a/b"} {
		if rec := serve(s, "GET", from); rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != want {
			t.Errorf("GET %s = %d -> %q, want 308 -> %q", from, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}

// With a credential, a page with no session goes to /login?next= and data answers 401 JSON;
// the login screen and its files stay open.
func TestWithACredentialTheConsoleGatesPagesAndOpensTheLoginScreen(t *testing.T) {
	s := consoleServer(t, auth.Credential{User: "ana", Hash: "x", Secret: []byte("a-test-secret-with-more-than-32-bytes!!")})
	rec := serve(s, "GET", "/runs")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?next=%2Fruns" {
		t.Errorf("GET /runs = %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := serve(s, "GET", "/api/session"); rec.Code != 401 {
		t.Errorf("GET /api/session = %d, want 401", rec.Code)
	}
	for _, p := range []string{"/login", "/assets/app.css", "/api/config", "/api/messages"} {
		if rec := serve(s, "GET", p); rec.Code != 200 {
			t.Errorf("GET %s = %d with no session, want 200", p, rec.Code)
		}
	}
}
