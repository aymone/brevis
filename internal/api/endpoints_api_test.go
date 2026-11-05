package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func endpointPaths(body map[string]any) []string {
	var out []string
	for _, r := range body["rows"].([]any) {
		out = append(out, r.(map[string]any)["path"].(string))
	}
	return out
}

// THE LIST IS AN INDEX with its counts: how many endpoints, APIs, and which
// serve stale data by the catalog; rows are stale first.
func TestTheEndpointsJSONJudgesByTheCatalog(t *testing.T) {
	f := twoAPIs()
	stale := catalog.Endpoint{Name: "vendors", Path: "/v1/vendors", Tenant: "key", Reads: []string{"bigquery://acme/bronze/old"}}
	f.all = append(f.all, postgres.PublishedEndpoint{API: "metrics-api", Name: "vendors", Path: "/v1/vendors",
		Tenant: "key", Reads: stale.Reads, Endpoint: stale, PublishedAt: time.Now().Add(-time.Hour)})
	ui := dataUI(catalogFake{entries: catalogEntries()}).WithEndpoints(f)
	code, body := dataJSON(t, ui, "GET", "/api/endpoints", "", "")
	if code != http.StatusOK {
		t.Fatalf("%d", code)
	}
	if body["total"] != float64(3) || body["apis"] != float64(2) || body["stale"] != float64(1) {
		t.Errorf("summary = %v", body)
	}
	rows := body["rows"].([]any)
	first := rows[0].(map[string]any)
	if first["path"] != "/v1/vendors" || first["freshness"] != "stale" {
		t.Errorf("first = %v", first)
	}
	rb := first["rebuilt"].(map[string]any)
	if rb["by"] != "vendors · fetch" {
		t.Errorf("rebuilt = %v", rb)
	}
	var orders map[string]any
	for _, r := range rows {
		if r.(map[string]any)["path"] == "/v1/orders" {
			orders = r.(map[string]any)
		}
	}
	if orders["params"] != float64(1) || orders["required"] != float64(1) || len(orders["reads"].([]any)) != 2 || orders["rebuilt"] != nil {
		t.Errorf("orders = %v", orders)
	}
}

func TestTheEndpointsJSONFilters(t *testing.T) {
	ui := dataUI(catalogFake{}).WithEndpoints(twoAPIs())
	for target, want := range map[string]string{
		"/api/endpoints?api=sales-api":  "/v1/orders",
		"/api/endpoints?q=PERIOD":       "/v1/clicks",
		"/api/endpoints?q=customers":    "/v1/orders",
		"/api/endpoints?state=stale":    "",
		"/api/endpoints?state=nonsense": "/v1/clicks /v1/orders",
	} {
		_, body := dataJSON(t, ui, "GET", target, "", "")
		if got := strings.Join(endpointPaths(body), " "); got != want {
			t.Errorf("%s = %q, want %q", target, got, want)
		}
	}
}

// NOTHING PUBLISHED is empty arrays and a total of zero, never null.
func TestTheEndpointsJSONListsAreArrays(t *testing.T) {
	mux := http.NewServeMux()
	dataUI(catalogFake{}).WithEndpoints(endpointsFake{}).Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/endpoints", nil))
	for _, want := range []string{`"rows":[]`, `"caches":[]`, `"apiNames":[]`, `"total":0`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("no %s in %s", want, rec.Body.String())
		}
	}
}

// AN ENDPOINT'S DATA is its contract, in words where the server writes them:
// parameters with what they accept, limits, the tables it reads with the
// catalog's verdict, the stale warning with the writer's reason.
func TestTheEndpointJSONIsItsContractAndItsLineage(t *testing.T) {
	f := published()
	f.all[0].Reads = []string{"bigquery://acme/bronze/old"}
	f.all[0].Manifest = postgres.APIManifest{Connection: "warehouse", Dialect: "bigquery",
		Cache: &catalog.EndpointsCache{Type: "redis", TTL: "6h", AddrFrom: "BREVIS_CACHE_ADDR"}}
	ui := dataUI(catalogFake{entries: catalogEntries()}).WithEndpoints(f)
	code, body := dataJSON(t, ui, "GET", "/api/endpoints/metrics-api/clicks", "", "")
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, body)
	}
	stale := body["stale"].(map[string]any)
	if !strings.Contains(stale["reason"].(string), "missed its") || stale["target"] != "bigquery://acme/bronze/old" {
		t.Errorf("stale = %v", stale)
	}
	l := body["lineage"].([]any)[0].(map[string]any)
	if l["known"] != true || l["status"] != "stale" || l["what"] != "old" || l["kind"] != "bigquery" ||
		l["writer"].(map[string]any)["href"] != "/ingest/loads/vendors/fetch" || l["cacheTtl"] != "6 hours" {
		t.Errorf("lineage = %v", l)
	}
	c := body["cache"].(map[string]any)
	if c["state"] != "set" || c["ttl"] != "6 hours" || c["type"] != "Redis" || !strings.Contains(c["bump"].(string), "bronze.old") {
		t.Errorf("cache = %v", c)
	}
	p := body["params"].([]any)
	if len(p) != 2 || p[1].(map[string]any)["accepts"] != "asc, desc" || p[1].(map[string]any)["note"] != "Newest first by default." {
		t.Errorf("params = %v", p)
	}
	lim := body["limits"].(map[string]any)
	if lim["timeout"] != "10 seconds" || lim["maxRows"] != float64(500) || lim["maxBytes"] != float64(1<<30) {
		t.Errorf("limits = %v", lim)
	}
	if body["canRun"] != false || len(body["form"].([]any)) != 0 || body["keyed"] != true {
		t.Errorf("canRun %v form %v keyed %v", body["canRun"], body["form"], body["keyed"])
	}
	if !strings.HasPrefix(body["curlHead"].(string), "curl -H") || body["curlBase"] != "<api address>/v1/clicks" {
		t.Errorf("curl %v %v", body["curlHead"], body["curlBase"])
	}
}

func TestTheEndpointJSON404sInJSON(t *testing.T) {
	ui := dataUI(catalogFake{}).WithEndpoints(published())
	if code, body := dataJSON(t, ui, "GET", "/api/endpoints/metrics-api/nope", "", ""); code != http.StatusNotFound || body["error"] == nil {
		t.Errorf("%d %v", code, body)
	}
}

// TRY IT OFFERS A FORM only where the console can run the endpoint, with
// each parameter's default in it.
func TestTheEndpointJSONOffersTryItOnlyWhereItCanRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	_, body := dataJSON(t, tryUI(t, srv.URL, loggedIn), "GET", "/api/endpoints/metrics-api/clicks", "", "")
	form := body["form"].([]any)
	if body["canRun"] != true || len(form) != 2 || form[0].(map[string]any)["value"] != "last_7_days" || form[0].(map[string]any)["kind"] != "select" {
		t.Errorf("canRun %v form %v", body["canRun"], form)
	}
}

// RUN: a POST with the parameters as JSON; the declared ones go to the API
// with the console's key, an empty one and an undeclared one do not; the
// rows come back as strings.
func TestTheCallRunsTheEndpointAndReturnsTheRows(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("X-Brevis-Cache", "miss")
		_, _ = w.Write([]byte(`{"data":[{"day":"2026-10-12","clicks":412,"n":null}]}`))
	}))
	defer srv.Close()
	code, body := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", "/api/endpoints/metrics-api/clicks/call",
		"application/json", `{"params":{"period":"today","order":"","nope":"x"}}`)
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, body)
	}
	if got == nil || got.URL.Query().Get("period") != "today" || got.URL.Query().Has("order") || got.URL.Query().Has("nope") ||
		got.Header.Get("Authorization") != "Bearer console-key" {
		t.Fatalf("the API was asked %v", got)
	}
	if body["status"] != float64(200) || body["statusText"] != "OK" || body["cache"] != "miss" || body["error"] != "" {
		t.Errorf("answer = %v", body)
	}
	cols := body["columns"].([]any)
	row := body["rows"].([]any)[0].([]any)
	if len(cols) != 3 || len(row) != 3 || row[1] != "412" || row[2] != "null" {
		t.Errorf("columns %v row %v", cols, row)
	}
}

// A REFUSAL FROM THE API is shown in its words, with its status.
func TestTheCallRepeatsTheAPIsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"the parameter period is one of: today, yesterday"}`))
	}))
	defer srv.Close()
	code, body := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", "/api/endpoints/metrics-api/clicks/call", "application/json", `{"params":{"period":"x"}}`)
	if code != http.StatusOK || body["status"] != float64(400) || !strings.Contains(body["error"].(string), "period is one of") {
		t.Errorf("%d %v", code, body)
	}
}

// ONLY JSON, ONLY A LOGIN, ONLY AN API WITH AN ADDRESS: the same refusals as
// the page, in JSON; an API that cannot be reached is a 502.
func TestTheCallKeepsThePagesRefusals(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }))
	url := "/api/endpoints/metrics-api/clicks/call"
	if code, _ := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", url, "application/x-www-form-urlencoded", "period=today"); code != http.StatusUnsupportedMediaType {
		t.Errorf("a form post answered %d", code)
	}
	if code, _ := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", url, "application/json", "{"); code != http.StatusBadRequest {
		t.Errorf("a broken body answered %d", code)
	}
	if code, _ := dataJSON(t, tryUI(t, srv.URL, auth0()), "POST", url, "application/json", `{"params":{}}`); code != http.StatusForbidden {
		t.Errorf("an open console answered %d", code)
	}
	if code, _ := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", "/api/endpoints/metrics-api/nope/call", "application/json", `{}`); code != http.StatusNotFound {
		t.Errorf("an unpublished endpoint answered %d", code)
	}
	if asked {
		t.Error("the API was called by a refused request")
	}
	srv.Close()
	if code, body := dataJSON(t, tryUI(t, srv.URL, loggedIn), "POST", url, "application/json", `{"params":{}}`); code != http.StatusBadGateway || body["error"] == nil {
		t.Errorf("an unreachable API answered %d %v", code, body)
	}
}

func auth0() auth.Credential { return auth.Credential{} }
