package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// AND WHEN THE SERVICE IS THE PROBLEM, THE LINE IS SILENT.
//
// A rate limit, a busy service or an unreachable one is an operator's
// problem and not the reader's: it says nothing about the statement in the
// box, and a red line under somebody's half-written SQL that is really about
// a token is a line that teaches them to ignore the line.
func TestTheCostLineIsSilentAboutTheServicesOwnProblems(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusUnauthorized} {
		w, ui := browsing(t)
		w.status["/v1/estimate"] = code
		w.estimate = `{"error":"this service prices at most 120 statements a minute"}`

		mux := http.NewServeMux()
		ui.Registrar(mux)
		rec := httptest.NewRecorder()
		form := url.Values{"target": {probeTarget}, "q": {"SELECT 1"}}
		r := httptest.NewRequest(http.MethodPost, "/api/sql/estimate", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mux.ServeHTTP(rec, r)

		if rec.Code == http.StatusOK && strings.TrimSpace(rec.Body.String()) != "" {
			t.Errorf("a %d from the service drew something on the screen: %s", code, rec.Body)
		}
	}
}
