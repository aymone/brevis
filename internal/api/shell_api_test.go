package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"brevis.sh/brevis/internal/updates"
)

// THE BADGES AS JSON: failing, then in flight, for the React shell to poll.
func TestNavRunsJSONCountsFailingAndRunning(t *testing.T) {
	ui := withRuns(runsFake{counts: map[string]int{"running": 2, "queued": 1}}).WithFailing(failingFake{n: 4})
	rec, body := getJSON(t, ui, "/api/nav/runs", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if body["failing"] != float64(4) || body["running"] != float64(3) || body["attention"] != float64(0) {
		t.Errorf("badges = %v; want failing 4, running 3, attention 0", body)
	}
}

// A COUNT THAT CANNOT BE READ is a JSON error, so the poll keeps what it showed.
func TestNavRunsJSONReportsAnUnreadableCount(t *testing.T) {
	rec, body := getJSON(t, withRuns(runsFake{err: errors.New("down")}), "/api/nav/runs", nil)
	if rec.Code != http.StatusInternalServerError || body["error"] == nil {
		t.Errorf("%d %v; want 500 with an error", rec.Code, body)
	}
}

// THE RELEASE, for the version modal; 204 where none is known.
func TestReleaseIsServedWhenKnownAndAbsentWhenNot(t *testing.T) {
	src := releaseFake{r: updates.Release{Tag: "v0.41.0", URL: "https://example.test/r", Notes: "<p>x</p>"}, newer: true}
	rec, body := getJSON(t, versionUI(src), "/api/release", nil)
	if rec.Code != http.StatusOK || body["tag"] != "v0.41.0" || body["newer"] != true || body["notes"] != "<p>x</p>" {
		t.Errorf("%d %v", rec.Code, body)
	}
	mux := http.NewServeMux()
	versionUI(nil).Registrar(mux)
	none := httptest.NewRecorder()
	mux.ServeHTTP(none, httptest.NewRequest(http.MethodGet, "/api/release", nil))
	if none.Code != http.StatusNoContent || none.Body.Len() != 0 {
		t.Errorf("%d %q; want an empty 204", none.Code, none.Body.String())
	}
}
