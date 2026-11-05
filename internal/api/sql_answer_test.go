package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A GET IGNORES THE HEADER: it runs nothing and draws the workbench, because
// the answer to opening a screen is the screen.
func TestAGetIgnoresTheFragmentHeader(t *testing.T) {
	w, ui := browsing(t)
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodGet, "/sql", nil)
	r.Header.Set("X-Brevis-Fragment", "answer")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Error("a GET with the header did not draw the workbench")
	}
	for _, c := range w.calls {
		if c != "/v1/objects" {
			t.Errorf("a GET asked the warehouse %s", c)
		}
	}
}
