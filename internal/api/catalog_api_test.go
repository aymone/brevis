package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/api"
)

// dataJSON asks a Data JSON endpoint and decodes what it answers.
func dataJSON(t *testing.T, ui *api.UI, method, target, ctype, body string) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("%s %s answered %q, not JSON: %.80s", method, target, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s is not an object: %v", target, err)
	}
	return rec.Code, out
}

func rowTargets(body map[string]any) []string {
	var out []string
	for _, r := range body["rows"].([]any) {
		out = append(out, r.(map[string]any)["target"].(string))
	}
	return out
}

// THE CATALOG IS WORST FIRST with the legacy row only when asked, each status
// counted with the rest of the filter as it is, and the rows carry the
// decisions the server makes: status, who writes, queued, the split target.
func TestTheCatalogJSONIsWorstFirstWithItsCounts(t *testing.T) {
	ui := dataUI(catalogFake{entries: catalogEntries()})
	code, body := dataJSON(t, ui, "GET", "/api/catalog", "", "")
	if code != http.StatusOK {
		t.Fatalf("%d", code)
	}
	if got := strings.Join(rowTargets(body), " "); got != "bigquery://acme/bronze/old pubsub://acme/clicks gs://lake/raw/manual/" {
		t.Errorf("rows = %q", got)
	}
	counts := body["counts"].(map[string]any)
	if counts["all"] != float64(3) || counts["attention"] != float64(1) || counts["stale"] != float64(1) || counts["continuous"] != float64(1) {
		t.Errorf("counts = %v", counts)
	}
	first := body["rows"].([]any)[0].(map[string]any)
	if first["status"] != "stale" || first["where"] != "bigquery://acme/bronze/" || first["what"] != "old" ||
		first["writer"] != "vendors · fetch" || first["lastRows"] != float64(12) || len(first["recent"].([]any)) != 3 {
		t.Errorf("first row = %v", first)
	}
	if body["hasLegacy"] != true || len(body["kinds"].([]any)) != 3 {
		t.Errorf("kinds %v, legacy %v", body["kinds"], body["hasLegacy"])
	}
	_, body = dataJSON(t, ui, "GET", "/api/catalog?legacy=1", "", "")
	if !strings.Contains(strings.Join(rowTargets(body), " "), "bronze.payments") {
		t.Error("legacy=1 does not show the legacy row")
	}
}

// THE FILTERS ARE THE PAGE'S: status (attention is stale and late together),
// kind and the search over the table and its writers; a count does not
// disagree with the list its choice opens.
func TestTheCatalogJSONFilters(t *testing.T) {
	ui := dataUI(catalogFake{entries: catalogEntries()})
	for target, want := range map[string]string{
		"/api/catalog?status=attention": "bigquery://acme/bronze/old",
		"/api/catalog?kind=pubsub":      "pubsub://acme/clicks",
		"/api/catalog?q=EXPORT":         "gs://lake/raw/manual/",
		"/api/catalog?status=bogus":     "bigquery://acme/bronze/old pubsub://acme/clicks gs://lake/raw/manual/",
	} {
		_, body := dataJSON(t, ui, "GET", target, "", "")
		if got := strings.Join(rowTargets(body), " "); got != want {
			t.Errorf("%s = %q, want %q", target, got, want)
		}
	}
	_, body := dataJSON(t, ui, "GET", "/api/catalog?kind=pubsub", "", "")
	if body["counts"].(map[string]any)["all"] != float64(1) || body["total"] != float64(3) || body["shown"] != float64(1) {
		t.Errorf("counts under a kind: %v total %v shown %v", body["counts"], body["total"], body["shown"])
	}
}

// NO MATCH IS AN EMPTY ARRAY, never null.
func TestTheCatalogJSONListsAreArrays(t *testing.T) {
	ui := dataUI(catalogFake{})
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/catalog", nil))
	for _, want := range []string{`"rows":[]`, `"kinds":[]`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("no %s in %s", want, rec.Body.String())
		}
	}
}

func TestAFailingCatalogReadIsAJSON500(t *testing.T) {
	code, body := dataJSON(t, dataUI(catalogFake{err: errors.New("down")}), "GET", "/api/catalog", "", "")
	if code != http.StatusInternalServerError || body["error"] == nil {
		t.Errorf("%d %v", code, body)
	}
}

// A DESTINATION UNNAMED BY ITS MANIFEST opens the stream that writes it.
func TestAnUnnamedDestinationPointsAtItsStream(t *testing.T) {
	entries := append(catalogEntries(), catalogEntries()[1])
	entries[len(entries)-1].Target = ""
	_, body := dataJSON(t, dataUI(catalogFake{entries: entries}), "GET", "/api/catalog", "", "")
	for _, r := range body["rows"].([]any) {
		if row := r.(map[string]any); row["target"] == "" {
			if row["streamHref"] != "/ingest/streams/edge/v1/clicks" || row["status"] != "unidentified" {
				t.Errorf("unnamed row = %v", row)
			}
			return
		}
	}
	t.Error("no unnamed row")
}

// THE DRAWER: the writer and why it stands there (the sentence's key and values,
// in its timezone), the last loads each opening its run, who reads it.
func TestTheTargetJSONSaysWhyAndWhoReadsIt(t *testing.T) {
	code, body := dataJSON(t, drawerUI(), "GET", "/api/catalog/target?u="+url.QueryEscape("bigquery://acme/bronze/old"), "", "")
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, body)
	}
	if body["row"].(map[string]any)["status"] != "stale" {
		t.Errorf("row = %v", body["row"])
	}
	w := body["writers"].([]any)[0].(map[string]any)
	if w["name"] != "vendors · fetch" || w["status"] != "stale" || first(w["reason"])["k"] != "catalog.why.missed" ||
		w["href"] != "/ingest/loads/vendors/fetch" || w["cron"] == "" {
		t.Errorf("writer = %v", w)
	}
	bars := body["bars"].([]any)
	if len(bars) != 2 || bars[0].(map[string]any)["href"] != "/runs/aaaaaaaa-0000-0000-0000-000000000002" {
		t.Errorf("bars = %v", bars)
	}
	rb := body["readBy"].([]any)
	if len(rb) != 1 || rb[0].(map[string]any)["path"] != "/v1/old" || rb[0].(map[string]any)["api"] != "metrics-api" {
		t.Errorf("readBy = %v", rb)
	}
	if body["configs"] == nil {
		t.Error("configs must be an array")
	}
}

// A DESTINATION NOBODY WRITES is a JSON 404; one that cannot be a target is a 400.
func TestTheTargetJSONRefusesInJSON(t *testing.T) {
	if code, body := dataJSON(t, drawerUI(), "GET", "/api/catalog/target?u="+url.QueryEscape("postgres://nowhere/x"), "", ""); code != http.StatusNotFound || body["error"] == nil {
		t.Errorf("unknown: %d %v", code, body)
	}
	for _, target := range []string{"/api/catalog/target", "/api/catalog/target?u=" + strings.Repeat("x", 5000)} {
		if code, _ := dataJSON(t, drawerUI(), "GET", target, "", ""); code != http.StatusBadRequest {
			t.Errorf("%.40s answered %d", target, code)
		}
	}
}
