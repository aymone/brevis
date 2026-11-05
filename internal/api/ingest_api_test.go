package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// ingestJSON asks the JSON endpoint and decodes an object.
func ingestJSON(t *testing.T, ui *api.UI, target string) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("%s answered %q, want JSON", target, rec.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s is not a JSON object: %v\n%s", target, err, rec.Body.String())
	}
	return rec.Code, body
}

func list(t *testing.T, body map[string]any, key string) []any {
	t.Helper()
	l, ok := body[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v; want an array, never null", key, body[key])
	}
	return l
}

// THE LOADS LIST is an array of rows with the counts beside it, and an empty
// installation is an empty array, not null.
func TestTheLoadsListIsAlwaysAnArray(t *testing.T) {
	code, body := ingestJSON(t, dataUI(catalogFake{}), "/api/ingest/loads")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	for _, k := range []string{"rows", "sources"} {
		if len(list(t, body, k)) != 0 {
			t.Errorf("%s is not empty", k)
		}
	}
	if body["total"] != float64(0) {
		t.Errorf("total = %v", body["total"])
	}
}

func TestTheLoadsListCarriesTheRowsAndTheirVerdict(t *testing.T) {
	code, body := ingestJSON(t, dataUI(loadCatalog()), "/api/ingest/loads")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	rows := list(t, body, "rows")
	if len(rows) != 4 {
		t.Fatalf("%d rows, want 4 (report, dump, 2 of split)", len(rows))
	}
	var report map[string]any
	for _, r := range rows {
		if m := r.(map[string]any); m["step"] == "report" {
			report = m
		}
	}
	if report == nil {
		t.Fatal("no report row")
	}
	if report["workflow"] != "daily_sales" || report["href"] != "/ingest/loads/daily_sales/report" ||
		report["status"] != "on_time" || keyOf(report["schedule"]) != "cron.daily" || report["rows"] != float64(4) {
		t.Errorf("report = %v", report)
	}
	if _, err := time.Parse(time.RFC3339, report["lastLoaded"].(string)); err != nil {
		t.Errorf("lastLoaded is not RFC 3339: %v", report["lastLoaded"])
	}
	if list(t, report, "recent") == nil {
		t.Error("recent is null")
	}
	// A step that writes two destinations links to the one asked.
	var many string
	for _, r := range rows {
		if m := r.(map[string]any); m["step"] == "split" {
			many = m["href"].(string)
		}
	}
	if !strings.Contains(many, "?target=") {
		t.Errorf("a step writing several destinations links to %q, without the target", many)
	}
	if body["workflows"] != float64(3) || body["total"] != float64(4) {
		t.Errorf("total %v workflows %v", body["total"], body["workflows"])
	}
}

// THE FILTER IS THE PAGE'S: a search narrows the rows and keeps the counts of
// what is held.
func TestTheLoadsListIsFilteredOnTheServer(t *testing.T) {
	_, body := ingestJSON(t, dataUI(loadCatalog()), "/api/ingest/loads?q=daily")
	rows := list(t, body, "rows")
	if len(rows) != 1 || rows[0].(map[string]any)["workflow"] != "daily_sales" {
		t.Errorf("rows = %v", rows)
	}
	if body["total"] != float64(4) {
		t.Errorf("total = %v; the summary describes every load, whatever is searched", body["total"])
	}
	if f := body["filter"].(map[string]any); f["q"] != "daily" {
		t.Errorf("filter = %v", f)
	}
}

func TestAFailingLoadsReadIsAJSON500(t *testing.T) {
	code, body := ingestJSON(t, dataUI(catalogFake{err: http.ErrAbortHandler}), "/api/ingest/loads")
	if code != http.StatusInternalServerError || body["error"] == nil {
		t.Errorf("%d %v; want a JSON 500 with an error", code, body)
	}
}

// ONE LOAD: this step's loads only, the bars linking to their runs, and a step
// that never described itself says so.
func TestALoadIsOneStepOnOneDestination(t *testing.T) {
	code, body := ingestJSON(t, dataUI(loadCatalog()), "/api/ingest/loads/daily_sales/report")
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}
	d, _ := body["detail"].(map[string]any)
	if d == nil || body["choosing"] != false {
		t.Fatalf("no detail: %v", body)
	}
	if d["status"] != "on_time" || keyOf(d["when"]) != "cron.daily" || d["described"] != false || d["config"] != nil {
		t.Errorf("detail = %v", d)
	}
	if tg := d["target"].(map[string]any); tg["uri"] != "file:///data/reports/" {
		t.Errorf("target = %v", tg)
	}
	loads := list(t, d, "loads")
	if len(loads) != 2 {
		t.Fatalf("%d loads, want this step's 2 (backfill's stays off)", len(loads))
	}
	if loads[0].(map[string]any)["bytes"] != float64(149) {
		t.Errorf("load = %v", loads[0])
	}
	bars := list(t, d, "bars")
	if len(bars) != 2 || bars[0].(map[string]any)["runId"] != "6420b0fd-0000-0000-0000-000000000002" {
		t.Errorf("bars = %v (oldest first, by run id)", bars)
	}
	for _, k := range []string{"changes", "readBy"} {
		list(t, d, k)
	}
	if strings.Contains(mustJSON(t, d), "backfill") {
		t.Error("another workflow's load leaked into this step")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A STEP THAT WRITES SEVERAL DESTINATIONS asks which; ?target= opens one. One
// that writes nothing, or not that destination, is a JSON 404.
func TestALoadWithSeveralDestinationsAsksWhich(t *testing.T) {
	cat := loadCatalog()
	cat.detail = &postgres.TargetDetail{CatalogEntry: cat.entries[2]}
	ui := dataUI(cat)
	_, body := ingestJSON(t, ui, "/api/ingest/loads/multi/split")
	if body["choosing"] != true || body["detail"] != nil || len(list(t, body, "choices")) != 2 {
		t.Errorf("choosing = %v detail = %v choices = %v", body["choosing"], body["detail"], body["choices"])
	}
	_, body = ingestJSON(t, ui, "/api/ingest/loads/multi/split?target="+url.QueryEscape("s3://lake/b/"))
	if body["choosing"] != false || body["detail"] == nil {
		t.Errorf("an open destination has no detail: %v", body)
	}
	for _, path := range []string{"/api/ingest/loads/multi/nothing", "/api/ingest/loads/multi/split?target=s3%3A%2F%2Felsewhere%2F"} {
		if code, b := ingestJSON(t, ui, path); code != http.StatusNotFound || b["error"] == nil {
			t.Errorf("%s answered %d %v; want a JSON 404", path, code, b)
		}
	}
}

// A DESCRIBED STEP carries its configuration in the tabs' grouping, finished.
func TestADescribedLoadCarriesItsConfiguration(t *testing.T) {
	cat := loadCatalog()
	cfg := &postgres.WriterConfig{Fields: []catalog.ManifestField{
		{Path: "source.driver", Value: "from.HTTP"}, {Path: "source.url", Value: "https://api.example.com/sales"},
		{Path: "transform.0.rename", Value: []any{"a", "b"}},
		{Path: "target.driver", Value: "files"}, {Path: "target.dedup", Value: "id"},
	}}
	cat.steps = map[postgres.StepKey]*postgres.WriterConfig{{Workflow: "daily_sales", Node: "report", Target: "file:///data/reports/"}: cfg}
	_, body := ingestJSON(t, dataUI(cat), "/api/ingest/loads/daily_sales/report")
	d := body["detail"].(map[string]any)
	c, _ := d["config"].(map[string]any)
	if d["described"] != true || c == nil {
		t.Fatalf("detail = %v", d)
	}
	if c["summary"] == "" || c["flow"] == nil {
		t.Errorf("the summary or the flow is missing: %v", c)
	}
	stage := c["stage"].(map[string]any)
	if stage["sourceDriver"] != "from.HTTP" || stage["targetDriver"] != "files" || stage["dedup"] != "id" || stage["transformCount"] != float64(1) {
		t.Errorf("stage = %v", stage)
	}
	if src := d["source"].(map[string]any); src["kind"] != "http" || src["where"] != "https://api.example.com/sales" {
		t.Errorf("source = %v", src)
	}
	for _, k := range []string{"other", "sourceHead", "common", "varying", "sources", "transform", "destination", "schema", "schemaExtra"} {
		list(t, c, k)
	}
}

func streamsCatalog() catalogFake {
	cfg := &postgres.WriterConfig{Fields: []catalog.ManifestField{{Path: "format", Value: "ndjson"}, {Path: "sink.type", Value: "pubsub"},
		{Path: "buffer.flush.every", Value: "1m"}, {Path: "dead_letter.type", Value: "files"}, {Path: "name", Value: "clicks"}}}
	c := catalogFake{entries: gatewayCatalog(), streams: map[postgres.StreamKey]*postgres.WriterConfig{{Gateway: "edge", Path: "/v1/clicks"}: cfg}}
	return c
}

func TestTheStreamsListIsAlwaysAnArray(t *testing.T) {
	_, body := ingestJSON(t, dataUI(catalogFake{}), "/api/ingest/streams")
	if len(list(t, body, "rows")) != 0 || len(list(t, body, "kinds")) != 0 {
		t.Errorf("body = %v", body)
	}
}

func TestTheStreamsListCarriesTheBatchAndTheDeadLetter(t *testing.T) {
	_, body := ingestJSON(t, dataUI(streamsCatalog()), "/api/ingest/streams?q=clicks")
	rows := list(t, body, "rows")
	if len(rows) == 0 {
		t.Fatalf("no rows: %v", body)
	}
	row := rows[0].(map[string]any)
	if row["gateway"] != "edge" || row["path"] != "/v1/clicks" || row["href"] != "/ingest/streams/edge/v1/clicks" || keyOf(row["batch"]) != "summary.unit.minute" {
		t.Errorf("row = %v", row)
	}
	if row["sink"].(map[string]any)["uri"] == "" {
		t.Errorf("sink = %v", row["sink"])
	}
	_, none := ingestJSON(t, dataUI(streamsCatalog()), "/api/ingest/streams?q=zzz-nothing")
	if len(list(t, none, "rows")) != 0 || none["total"] == float64(0) {
		t.Errorf("a search that finds nothing keeps the totals: %v", none)
	}
}

func TestAStreamCarriesItsFlowSettingsAndDeadLetter(t *testing.T) {
	ui := dataUI(streamsCatalog())
	code, body := ingestJSON(t, ui, "/api/ingest/streams/edge/v1/clicks")
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}
	if body["gateway"] != "edge" || body["path"] != "/v1/clicks" || body["format"] != "ndjson" || body["hasConfig"] != true {
		t.Errorf("stream = %v", body)
	}
	if body["lede"] == "" || body["summary"] == "" || body["flow"] == nil || body["yaml"] == "" {
		t.Errorf("a sentence is missing: %v", body)
	}
	if len(list(t, body, "cards")) == 0 {
		t.Error("the settings are not in cards")
	}
	te := body["testEvent"].(map[string]any)
	if te["format"] != "ndjson" || !strings.Contains(te["curlHead"].(string), "/v1/clicks") {
		t.Errorf("testEvent = %v", te)
	}
	// What an auto_table's event is checked by is always a list and a string, never null.
	if _, ok := te["allow"].([]any); !ok || te["dataset"] == nil {
		t.Errorf("testEvent has no allow list or dataset: %v", te)
	}
	for _, k := range []string{"tables", "readBy", "changes"} {
		list(t, body, k)
	}
	if code, b := ingestJSON(t, ui, "/api/ingest/streams/edge/v1/nothing"); code != http.StatusNotFound || b["error"] == nil {
		t.Errorf("an unknown stream answered %d %v; want a JSON 404", code, b)
	}
}

// keyOf is the catalog key of the first message in a value the server sent for the app to say: a message, or a
// "cat" of messages and strings. The server sends keys and data, never a sentence.
func keyOf(v any) string {
	m, _ := v.(map[string]any)
	if parts, ok := m["v"].([]any); ok {
		for _, p := range parts {
			if k := keyOf(p); k != "" {
				return k
			}
		}
		return ""
	}
	k, _ := m["k"].(string)
	return k
}
