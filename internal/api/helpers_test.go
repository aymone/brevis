package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"brevis.sh/brevis/internal/api"
)

func pageIn(t *testing.T, ui *api.UI, path, cookie, accept string, login bool) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	if login {
		_ = mux
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "brevis_lang", Value: cookie})
	}
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	raw, _ := io.ReadAll(rec.Result().Body)
	return string(raw)
}

// THE PAGE SAYS WHICH LANGUAGE IT IS IN, the one the server chose: a screen
// reader pronounces by it, and a browser offers to translate by it.
