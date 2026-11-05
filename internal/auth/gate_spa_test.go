package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/auth"
)

func spaGate(t *testing.T) (*auth.Gate, *[]string) {
	t.Helper()
	var arrived []string
	return &auth.Gate{
		Cred: credential(t, "operador", "senha-de-teste-longa"),
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { arrived = append(arrived, r.URL.Path) }),
	}, &arrived
}

// Exactly what the React login screen needs is public, and a near miss of each
// path is not.
func TestOnlyTheLoginScreensPathsAreOpenWithNoSession(t *testing.T) {
	gate, arrived := spaGate(t)
	open := map[string]string{
		"/login": "GET", "/static/index-abc.js": "GET", "/static/x/y.css": "GET", "/assets/app.css": "GET",
		"/api/config": "GET", "/api/messages": "GET", "/api/login": "POST", "/api/logout": "POST",
	}
	for p, m := range open {
		gate.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, p, nil))
	}
	if len(*arrived) != len(open) {
		t.Errorf("%v got through; expected %d open paths", *arrived, len(open))
	}

	*arrived = nil
	for _, p := range []string{
		"/", "/runs", "/login/x", "/loginx", "/static", "/index.html", "/logout", "/app", "/app/login", "/app/assets/x.js",
		"/api/session", "/api/loginx", "/api/config/x", "/api/projects", "/api/release", "/api/runs/1/graph",
	} {
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s = %d with no session", p, rec.Code)
		}
	}
	if len(*arrived) != 0 {
		t.Errorf("gated paths reached the handler: %v", *arrived)
	}
}

func TestAnAPIRequestWithNoSessionIs401JSONForEveryMethod(t *testing.T) {
	gate, arrived := spaGate(t)
	for _, m := range []string{"GET", "POST", "PUT", "DELETE"} {
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, httptest.NewRequest(m, "/api/session", nil))
		if rec.Code != 401 || strings.TrimSpace(rec.Body.String()) != `{"error":"sign in"}` ||
			!strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Header().Get("Location") != "" {
			t.Errorf("%s: %d %q %v", m, rec.Code, rec.Body, rec.Header())
		}
	}
	if len(*arrived) != 0 {
		t.Error("reached the handler")
	}
}

func TestAPageWithNoSessionGoesToTheLoginAndComesBackThroughNext(t *testing.T) {
	gate, _ := spaGate(t)
	for p, want := range map[string]string{
		"/runs?state=failed&page=2": "/login?next=%2Fruns%3Fstate%3Dfailed%26page%3D2",
		"/":                         "/login",
		"/ingest/streams/gw/v1/a.b": "/login?next=%2Fingest%2Fstreams%2Fgw%2Fv1%2Fa.b",
		"/app/runs":                 "/login?next=%2Fapp%2Fruns",
	} {
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("GET %s = %d -> %q, want 303 -> %q", p, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	// A non-API POST keeps the plain 401.
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, httptest.NewRequest("POST", "/workflows/x/trigger", nil))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "session expired") {
		t.Errorf("POST = %d %q", rec.Code, rec.Body)
	}
}

func TestAValidSessionStillReachesAnAPIPath(t *testing.T) {
	cred := credential(t, "operador", "senha-de-teste-longa")
	var user string
	gate := &auth.Gate{Cred: cred, Insecure: true, Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { user = auth.De(r.Context()) })}
	rec := httptest.NewRecorder()
	if !gate.SignIn(rec, "operador", "senha-de-teste-longa") {
		t.Fatal("sign-in refused")
	}
	req := httptest.NewRequest("GET", "/api/session", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	gate.ServeHTTP(httptest.NewRecorder(), req)
	if user != "operador" {
		t.Errorf("user = %q", user)
	}
}

// A PUBLIC PATH KNOWS WHO IS ASKING: an anonymous visitor is marked, one with a
// session carries the operator, and an open console marks nobody. The config
// endpoint uses it to keep the engine's version from a stranger.
func TestAPublicPathTellsAStrangerFromAnOperator(t *testing.T) {
	var anonymous bool
	var who string
	gate := &auth.Gate{
		Cred: credential(t, "operador", "senha-de-teste-longa"),
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			anonymous, who = auth.Anonymous(r.Context()), auth.De(r.Context())
		}),
	}
	gate.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/config", nil))
	if !anonymous || who != "" {
		t.Errorf("a stranger: anonymous=%v operator=%q; want true and none", anonymous, who)
	}
	login := httptest.NewRecorder()
	if !gate.SignIn(login, "operador", "senha-de-teste-longa") {
		t.Fatal("the right credential was refused")
	}
	req := httptest.NewRequest("GET", "/api/config", nil)
	req.AddCookie(login.Result().Cookies()[0])
	gate.ServeHTTP(httptest.NewRecorder(), req)
	if anonymous || who != "operador" {
		t.Errorf("an operator on a public path: anonymous=%v operator=%q", anonymous, who)
	}
	forged := httptest.NewRequest("GET", "/api/config", nil)
	forged.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "operador.9999999999.forged"})
	gate.ServeHTTP(httptest.NewRecorder(), forged)
	if !anonymous {
		t.Error("a forged cookie on a public path was not treated as a stranger")
	}
	var open bool
	openGate := &auth.Gate{Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { open = auth.Anonymous(r.Context()) })}
	openGate.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/config", nil))
	if open {
		t.Error("an open console marked its visitor as a stranger")
	}
}
