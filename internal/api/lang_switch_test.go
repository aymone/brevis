package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func switchLang(t *testing.T, lang, next string) *http.Response {
	t.Helper()
	mux := http.NewServeMux()
	dataUI(catalogFake{}).Registrar(mux)
	form := url.Values{"lang": {lang}, "next": {next}}
	req := httptest.NewRequest(http.MethodPost, "/preferences/lang", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

// CHOOSING A LANGUAGE IS A POST THAT REMEMBERS IT AND GOES BACK, with no
// script: the cookie for a year, and a 303 to the page the reader was on.
func TestChoosingALanguageRemembersItAndReturnsToThePage(t *testing.T) {
	res := switchLang(t, "pt-BR", "/runs?status=failed")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/runs?status=failed" {
		t.Fatalf("%d to %q", res.StatusCode, res.Header.Get("Location"))
	}
	var got *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "brevis_lang" {
			got = c
		}
	}
	if got == nil || got.Value != "pt-BR" || got.Path != "/" || got.MaxAge < 365*24*3600 || got.SameSite != http.SameSiteLaxMode {
		t.Errorf("the cookie is %+v", got)
	}
}

// THE WAY BACK IS ONLY EVER THIS CONSOLE: `next` goes through auth.Target, the
// login's own guard, so the switcher cannot be made into an open redirect.
func TestTheWayBackNeverLeavesTheConsole(t *testing.T) {
	for _, next := range []string{"//evil.example", "https://evil.example/", `/\evil.example`, ""} {
		if loc := switchLang(t, "es", next).Header.Get("Location"); loc != "/" {
			t.Errorf("next %q sent the reader to %q", next, loc)
		}
	}
}

// A LANGUAGE THE CONSOLE DOES NOT SPEAK IS REFUSED, and nothing is written.
func TestAnUnknownLanguageIsRefused(t *testing.T) {
	res := switchLang(t, "it", "/runs")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d", res.StatusCode)
	}
	if len(res.Cookies()) > 0 {
		t.Errorf("a cookie was written: %v", res.Cookies())
	}
}
