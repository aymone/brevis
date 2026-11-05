package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

func sessionUI(brand branding.Brand) *api.UI {
	return api.NewUI(nil, nil, nil, nil, nil, catalogFake{}, brand, slog.New(slog.DiscardHandler)).
		WithVersion("1.2.3", nil)
}

func getJSON(t *testing.T, ui *api.UI, target string, req func(*http.Request)) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if req != nil {
		req(r)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", target, err, rec.Body.String())
	}
	return rec, body
}

// THE APP ASKS WHO IS LOOKING before it draws a thing: the operator the Gate
// put in the context and the language the cookie chose.
func TestTheSessionNamesTheOperatorAndTheLanguage(t *testing.T) {
	rec, body := getJSON(t, sessionUI(branding.Default()), "/api/session", func(r *http.Request) {
		*r = *r.WithContext(auth.IntoContext(context.Background(), "ana"))
		r.AddCookie(&http.Cookie{Name: i18n.Cookie, Value: "pt-BR"})
	})
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d, cache %q; a session is never cached", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if body["operator"] != "ana" || body["lang"] != "pt-BR" {
		t.Errorf("session = %v", body)
	}
}

// AN OPEN CONSOLE has no operator: the field is empty, not absent, so the app
// reads one shape.
func TestAnOpenConsoleHasAnEmptyOperator(t *testing.T) {
	_, body := getJSON(t, sessionUI(branding.Default()), "/api/session", nil)
	if op, ok := body["operator"]; !ok || op != "" {
		t.Errorf("operator = %v (present %v); want an empty string", op, ok)
	}
}

// THE CONFIG IS THE INSTALLATION'S, the same for everyone: the brand (its logo
// falling back to the embedded mark, its theme the very CSS the pages inject),
// the version and whether there is SQL.
func TestTheConfigCarriesTheBrandTheVersionAndTheServices(t *testing.T) {
	brand := branding.Default()
	brand.Title = "Acme Ops"
	brand.Theme.Accent = "#ff6600"
	rec, body := getJSON(t, sessionUI(brand), "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	b, _ := body["brand"].(map[string]any)
	if b["title"] != "Acme Ops" || b["logo"] != branding.DefaultLogo || b["attribution"] != branding.Attribution {
		t.Errorf("brand = %v", b)
	}
	if b["css"] != brand.CSS() || b["css"] == "" {
		t.Errorf("css = %q; want the pages' theme %q", b["css"], brand.CSS())
	}
	if body["version"] != "1.2.3" || body["sql"] != false {
		t.Errorf("config = %v", body)
	}
}

type projectsFake struct {
	api.Leitura
	ps  []postgres.ProjectSummary
	err error
}

func (f projectsFake) Projects(context.Context) ([]postgres.ProjectSummary, error) {
	return f.ps, f.err
}

func projectsUI(f projectsFake) *api.UI {
	return api.NewUI(f, nil, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}

// THE PROJECTS ARE AN ARRAY, EMPTY INCLUDED: `[]`, never `null`, so the screen
// has one shape to read.
func TestTheProjectsAreAlwaysAnArray(t *testing.T) {
	mux := http.NewServeMux()
	projectsUI(projectsFake{}).Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("%d %q; want 200 and []", rec.Code, rec.Body.String())
	}
}

func TestTheProjectsListTheirNumbers(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	projectsUI(projectsFake{ps: []postgres.ProjectSummary{{Slug: "sales", Name: "Sales", Workflows: 3, Runs: 40, CreatedAt: at}}}).Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if got[0]["slug"] != "sales" || got[0]["workflows"] != float64(3) || got[0]["runs"] != float64(40) || got[0]["createdAt"] != "2026-10-01T09:00:00Z" {
		t.Errorf("project = %v", got[0])
	}
}

// A READ THAT FAILS IS A 500 IN JSON, not an HTML error page the app cannot parse.
func TestAFailingProjectsReadIsAJSON500(t *testing.T) {
	mux := http.NewServeMux()
	projectsUI(projectsFake{err: errors.New("down")}).Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Errorf("%d %q; want a JSON 500", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// THE CONFIG IS PUBLIC (the sign-in screen draws the brand from it), so a
// stranger gets the brand and neither the engine's version nor the services; a
// signed-in operator gets everything. Through the real Gate, not a stand-in.
func TestTheConfigHidesTheInstallationFromAStranger(t *testing.T) {
	hash, err := auth.GenerateHash("senha-de-teste-longa")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	sessionUI(branding.Default()).Registrar(mux)
	gate := &auth.Gate{
		Cred: auth.Credential{User: "ana", Hash: hash, Secret: []byte("um-segredo-de-teste-com-mais-de-32-bytes")},
		Next: mux,
	}
	get := func(cookie *http.Cookie) map[string]any {
		r := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("/api/config answered %d: the sign-in screen needs it with no session", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	stranger := get(nil)
	if brand, _ := stranger["brand"].(map[string]any); brand["title"] == nil || brand["title"] == "" {
		t.Errorf("a stranger lost the brand: %v", stranger)
	}
	if stranger["version"] != "" || stranger["sql"] != false {
		t.Errorf("a stranger saw the installation: version %q sql %v", stranger["version"], stranger["sql"])
	}
	login := httptest.NewRecorder()
	if !gate.SignIn(login, "ana", "senha-de-teste-longa") {
		t.Fatal("the right credential was refused")
	}
	if signedIn := get(login.Result().Cookies()[0]); signedIn["version"] != "1.2.3" {
		t.Errorf("an operator did not get the version: %v", signedIn["version"])
	}
}
