package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/api"
)

func sqlJSON(t *testing.T, ui *api.UI, method, path, body string) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s is not JSON: %v\n%s", method, path, err, rec.Body)
		}
	}
	return rec.Code, out
}

func TestSQLStateNamesConnectionsTreeAndWrites(t *testing.T) {
	w, ui := browsing(t)
	w.objects = `{"relations":[{"schema":"sales","name":"daily"}],"writable":true}`
	code, got := sqlJSON(t, ui, "GET", "/api/sql/state?target=bigquery://acme-prod/bronze/orders", "")
	if code != 200 || got["target"] != probeTarget {
		t.Fatalf("%d %v", code, got)
	}
	if got["preloaded"] != true || !strings.Contains(got["statement"].(string), "bronze.orders") {
		t.Errorf("first look missing: %v", got)
	}
	tree := got["tree"].(map[string]any)
	if tree["writable"] != true || len(tree["schemas"].([]any)) != 1 {
		t.Errorf("tree = %v", tree)
	}
	if len(got["connections"].([]any)) != 1 {
		t.Errorf("connections = %v", got["connections"])
	}
}

func TestSQLJSONDoorsAnswer404WithoutAServiceOrForAnUnknownTarget(t *testing.T) {
	code, got := sqlJSON(t, openConsole(t, nil), "GET", "/api/sql/state", "")
	if code != 404 || got["error"] == nil {
		t.Errorf("no service: %d %v", code, got)
	}
	_, ui := browsing(t)
	for _, c := range []struct{ m, p, b string }{
		{"GET", "/api/sql/tree?target=bigquery://nope/x/y", ""},
		{"GET", "/api/sql/describe?target=bigquery://nope/x/y&schema=a&name=b", ""},
		{"POST", "/api/sql/run", `{"target":"bigquery://nope/x/y","q":"SELECT 1"}`},
		{"POST", "/api/sql/cost", `{"target":"bigquery://nope/x/y","q":"SELECT 1"}`},
	} {
		if code, got := sqlJSON(t, ui, c.m, c.p, c.b); code != 404 || got["error"] == nil {
			t.Errorf("%s %s: %d %v", c.m, c.p, code, got)
		}
	}
}

func TestSQLDescribeListsColumns(t *testing.T) {
	_, ui := browsing(t)
	code, got := sqlJSON(t, ui, "GET", "/api/sql/describe?target="+probeTarget+"&schema=sales&name=daily", "")
	cols, _ := got["columns"].([]any)
	if code != 200 || len(cols) != 2 {
		t.Fatalf("%d %v", code, got)
	}
	if code, _ := sqlJSON(t, ui, "GET", "/api/sql/describe?target="+probeTarget, ""); code != 400 {
		t.Errorf("missing relation answered %d", code)
	}
}

func TestSQLRunReturnsTheGridAndCarriesTheLimit(t *testing.T) {
	w, ui := browsing(t)
	w.query = `{"columns":["sku","n"],"rows":[["W",1],[null,2]],"truncated":true,"limit":100,"bytes":2048,"ms":31}`
	code, got := sqlJSON(t, ui, "POST", "/api/sql/run", `{"target":"`+probeTarget+`","q":"  SELECT sku FROM t  "}`)
	if code != 200 || got["truncated"] != true || got["ms"] != float64(31) || len(got["rows"].([]any)) != 2 {
		t.Fatalf("%d %v", code, got)
	}
	if w.asked["/v1/query"]["statement"] != "SELECT sku FROM t" || w.asked["/v1/query"]["limit"] != float64(100) {
		t.Errorf("service was asked %v", w.asked["/v1/query"])
	}
}

func TestSQLRunFailuresAreJSONWithAKind(t *testing.T) {
	w, ui := browsing(t)
	body := `{"target":"` + probeTarget + `","q":"DELETE FROM t"}`

	w.status["/v1/query"], w.query = 400, `{"error":"this service only reads"}`
	if code, got := sqlJSON(t, ui, "POST", "/api/sql/run", body); code != 422 || got["kind"] != "refused" || got["error"] != "this service only reads" {
		t.Errorf("refused: %d %v", code, got)
	}
	w.query = `{"error":"x","code":"no-connection"}`
	if code, got := sqlJSON(t, ui, "POST", "/api/sql/run", body); code != 422 || got["kind"] != "no_connection" || got["connection"] != "acme-prod" {
		t.Errorf("no connection: %d %v", code, got)
	}
	w.status["/v1/query"] = 500
	if code, got := sqlJSON(t, ui, "POST", "/api/sql/run", body); code != 502 || got["kind"] != "failed" {
		t.Errorf("failed: %d %v", code, got)
	}
	if code, got := sqlJSON(t, ui, "POST", "/api/sql/run", `{"target":"`+probeTarget+`","q":"  "}`); code != 400 || got["kind"] != "empty" {
		t.Errorf("empty: %d %v", code, got)
	}
	if code, _ := sqlJSON(t, ui, "POST", "/api/sql/run", `{"target":"`+probeTarget+`","q":"`+strings.Repeat("a", 70<<10)+`"}`); code != 400 {
		t.Errorf("too long: %d", code)
	}
}

func TestSQLRunIsAPostAndCostIsPricedOrSilent(t *testing.T) {
	w, ui := browsing(t)
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sql/run?q=SELECT+1", nil))
	if rec.Code != 404 { // no GET route: the console falls through to its JSON 404
		t.Errorf("GET run answered %d", rec.Code)
	}
	w.estimate = `{"State":"priced","Bytes":124723}`
	code, got := sqlJSON(t, ui, "POST", "/api/sql/cost", `{"target":"`+probeTarget+`","q":"SELECT 1"}`)
	if code != 200 || got["state"] != "priced" || got["bytes"] != float64(124723) {
		t.Errorf("cost: %d %v", code, got)
	}
	if code, _ := sqlJSON(t, ui, "POST", "/api/sql/cost", `{"target":"`+probeTarget+`","q":""}`); code != 204 {
		t.Errorf("empty statement priced: %d", code)
	}
	w.status["/v1/estimate"] = 500
	w.estimate = `nope`
	if code, _ := sqlJSON(t, ui, "POST", "/api/sql/cost", `{"target":"`+probeTarget+`","q":"SELECT 1"}`); code != 204 {
		t.Errorf("unreachable service must be silent: %d", code)
	}
}
