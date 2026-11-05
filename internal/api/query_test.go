package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/infrastructure/sqlserve"
)

// sqlFake is a `brevis-sql serve` that remembers what it was sent.
type sqlFake struct {
	asked  map[string]any
	status int
	body   string
}

func (f *sqlFake) start(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.asked)
		if f.status == 0 {
			f.status = http.StatusOK
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(s.Close)
	return s
}

// The Query form's helper lived here and went with the form: a destination
// page posts nothing now, and `runSQL` in sql_test.go submits the workbench's
// own. Dead test code is still code, and the linter is what noticed.

// A CONSOLE WITH NO LOGIN DOES NOT OFFER THE WAREHOUSE.
//
// CHECKPOINT B's F4. The console can run with no credential at all, and
// already says so at boot: "interface is OPEN: anyone can trigger a
// workflow". With the Query tab that sentence became incomplete -- it is now
// also anyone can READ EVERY TABLE IN THE PROJECT, because a query is not
// confined to the destination it was opened from, by design.
//
// The tab does not create the hole. It changes what falls through it, and a
// data tool must not outlive the authentication of the screen it sits on.
func TestAnUnprotectedConsoleOffersNoTabs(t *testing.T) {
	asked := false
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked = true
		_, _ = w.Write([]byte(`{"columns":["k"],"rows":[["1"]]}`))
	}))
	defer svc.Close()
	open := openConsole(t, sqlserve.New(svc.URL, ""))

	body := render(t, open, "/ingest/loads/bronze/load")
	for _, gone := range []string{"/sql?target="} {
		if strings.Contains(body, gone) {
			t.Errorf("a console with no login offers %q", gone)
		}
	}

	// AND NOT BY HIDING A LINK. Somebody typing the parameter, or following
	// one from a message, must not reach a warehouse either.
	render(t, open, "/ingest/loads/bronze/load?tab=preview")
	if asked {
		t.Error("an unprotected console reached the warehouse")
	}
}
