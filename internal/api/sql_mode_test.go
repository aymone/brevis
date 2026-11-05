package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// screen opens /sql on a target and returns what was drawn.
func screen(t *testing.T, ui interface{ Registrar(*http.ServeMux) }, target string) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodGet, "/sql?target="+target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("/sql answered %d", rec.Code)
	}
	return rec.Body.String()
}

// AND A READ-ONLY WORKBENCH SAYS NOTHING, which is the default and every
// deployment that has not asked. A warning on every screen is a warning
// nobody reads.
func TestAReadOnlyWorkbenchDrawsNoBanner(t *testing.T) {
	w, ui := browsing(t)
	w.objects = `{"relations":[{"schema":"bronze","name":"orders"}]}`
	body := screen(t, ui, probeTarget)

	if strings.Contains(body, "data-writable") {
		t.Error("a read-only workbench warned about writes")
	}
	if strings.Contains(body, "change data") {
		t.Error("a read-only workbench says it may change data")
	}
}
