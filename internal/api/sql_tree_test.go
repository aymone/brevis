package api_test

import (
	"net/http"
	"testing"
)

// THE FRAGMENTS OBEY THE CATALOG TOO. They are a second door into the same
// service, and a rule enforced at one door is not a rule.
func TestTheFragmentsRefuseATargetTheCatalogDoesNotKnow(t *testing.T) {
	for _, path := range []string{
		"/api/sql/objects?target=bigquery://elsewhere/x/y",
		"/api/sql/columns?target=bigquery://elsewhere/x/y&schema=a&name=b",
	} {
		t.Run(path, func(t *testing.T) {
			w, ui := browsing(t)
			code, _ := get(t, ui, path)
			if code != http.StatusNotFound {
				t.Errorf("answered %d, wanted 404", code)
			}
			if len(w.calls) != 0 {
				t.Errorf("it reached the service anyway: %v", w.calls)
			}
		})
	}
}
