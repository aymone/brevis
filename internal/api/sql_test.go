package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/infrastructure/sqlserve"
)

// A DESTINATION DOES NOT QUERY IN PLACE: a query is not about one table --
// it joins, it groups, it reaches across -- and a tab that pretends
// otherwise is what made both screens worse. Its preview went too
// (screen-loads T4): the drawer is narrow, and the workbench is a click away.
func TestADestinationDoesNotQueryInPlace(t *testing.T) {
	svc := (&sqlFake{body: `{"columns":["k"],"rows":[["1"]]}`}).start(t)
	ui := consoleOf(t, sqlserve.New(svc.URL, ""), signedIn, probeTarget, "bigquery")

	body := render(t, ui, "/ingest/loads/bronze/load")
	if strings.Contains(body, "tab=query") || strings.Contains(body, "tab=preview") {
		t.Error("the destination still offers a tab of its own")
	}
}

// A STATEMENT IN THE URL IS NOT A STATEMENT, moved here with the form.
//
// This is the property the POST exists for, and it holds against a link
// somebody crafted rather than against the form the page draws: `?q=…` is
// ignored, so a query cannot be put in a browser's history, a proxy log or a
// Referer header by anybody -- including by a page on another site linking
// here. `PostFormValue` is what does it, by reading the body and never the
// query string.
func TestAStatementInTheWorkbenchURLIsIgnored(t *testing.T) {
	// RAN, not CONTACTED. The page asks this same service what the
	// connection holds on every render, so "the service was called" stopped
	// being the question the moment the tree arrived -- the question is
	// whether a STATEMENT crossed, and that is a field in the body.
	asked := false
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Query != "" {
			asked = true
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer svc.Close()
	ui := consoleOf(t, sqlserve.New(svc.URL, ""), signedIn, probeTarget, "bigquery")

	render(t, ui, "/sql?target="+url.QueryEscape(probeTarget)+"&q="+url.QueryEscape("SELECT 1"))
	if asked {
		t.Error("a statement in the URL was run")
	}

	// AND NOT EVEN WHEN THE REQUEST IS A POST.
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost,
		"/sql?q="+url.QueryEscape("SELECT 1")+"&target="+url.QueryEscape(probeTarget),
		strings.NewReader("target="+url.QueryEscape(probeTarget)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, r)
	if asked {
		t.Error("a POST took its statement from the URL")
	}
}

// warehouse is a `serve` that answers each endpoint differently and
// remembers what each was asked. The single-body fake above cannot tell a
// listing from a query, which stopped being enough the moment one page view
// called two endpoints.
type warehouse struct {
	objects  string
	columns  string
	query    string
	estimate string

	status map[string]int
	asked  map[string]map[string]any
	calls  []string
}

func (w *warehouse) start(t *testing.T) *httptest.Server {
	t.Helper()
	w.asked = map[string]map[string]any{}
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.asked[r.URL.Path] = body
		w.calls = append(w.calls, r.URL.Path)
		if code := w.status[r.URL.Path]; code != 0 {
			rw.WriteHeader(code)
		}
		switch r.URL.Path {
		case "/v1/objects":
			_, _ = rw.Write([]byte(w.objects))
		case "/v1/columns":
			_, _ = rw.Write([]byte(w.columns))
		case "/v1/estimate":
			_, _ = rw.Write([]byte(w.estimate))
		default:
			_, _ = rw.Write([]byte(w.query))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func browsing(t *testing.T) (*warehouse, *api.UI) {
	t.Helper()
	w := &warehouse{
		objects: `{"relations":[{"schema":"sales","name":"daily"},{"schema":"sales","name":"events"}]}`,
		columns: `{"columns":[{"name":"sku","type":"text"},{"name":"quantity","type":"integer"}]}`,
		query:   `{"columns":["n"],"rows":[["1"]]}`,
		status:  map[string]int{},
	}
	svc := w.start(t)
	return w, consoleOf(t, sqlserve.New(svc.URL, ""), signedIn, probeTarget, "bigquery")
}
