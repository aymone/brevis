package pages

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

// catalogNow is a fixed 11:20 UTC, and catalogFixture a destination in every
// status against an hourly writer: last loaded at 11:05 is on time, at 10:00
// late (the 11:00 slot went by), at 08:00 stale (09:00 and 10:00 did).
var catalogNow = time.Date(2026, 10, 30, 11, 20, 0, 0, time.UTC)

func clock(h, m int) time.Time { return time.Date(2026, 10, 30, h, m, 0, 0, time.UTC) }

func rows64(v int64) *int64 { return &v }

func hourlyWriter(workflow, node string, last time.Time, active bool) postgres.CatalogWriter {
	return postgres.CatalogWriter{Workflow: workflow, Node: node, LastLoaded: last, LastRows: rows64(10),
		HasSchedule: true, Cron: "0 * * * *", Timezone: "UTC", Active: active}
}

func catalogFixture() []postgres.CatalogEntry {
	return []postgres.CatalogEntry{
		{Target: "bigquery://acme/bronze/fresh", Kind: "bigquery", Writers: []postgres.CatalogWriter{hourlyWriter("orders", "load", clock(11, 5), true)}},
		{Target: "pubsub://acme/clicks", Kind: "pubsub", Writers: []postgres.CatalogWriter{{Gateway: &postgres.GatewayWriter{Name: "edge", Stream: "/v1/clicks", Role: "sink"}, LastLoaded: clock(11, 19)}}},
		{Target: "bigquery://acme/bronze/old", Kind: "bigquery", Writers: []postgres.CatalogWriter{hourlyWriter("vendors", "fetch", clock(8, 0), true)}},
		{Target: "postgres://db/public/invoices", Kind: "postgres", Writers: []postgres.CatalogWriter{hourlyWriter("billing", "sync", clock(10, 0), true)}},
		{Target: "gs://lake/raw/manual/", Kind: "gs", Writers: []postgres.CatalogWriter{{Workflow: "export", Node: "write", LastLoaded: clock(9, 0)}}},
		{Target: "bigquery://acme/analysis/profile", Kind: "bigquery", Writers: []postgres.CatalogWriter{hourlyWriter("profile", "run", clock(7, 0), false)}},
		{Target: "bronze.payments", Legacy: true, Writers: []postgres.CatalogWriter{{Workflow: "payments", Node: "load", LastLoaded: clock(6, 0)}}},
	}
}

func targetsOf(v CatalogScreen) string {
	var out []string
	for _, r := range v.Rows {
		out = append(out, r.Target)
	}
	return strings.Join(out, " ")
}

// THE CATALOG IS WORST FIRST (screen-catalog T1): stale, late, on time,
// continuous, paused, no schedule, unidentified -- and within a status, the
// most recently loaded first. A legacy row waits until it is asked for.
func TestTheCatalogIsWorstFirst(t *testing.T) {
	v := BuildCatalog(catalogFixture(), DataFilter{}, catalogNow)
	want := "bigquery://acme/bronze/old postgres://db/public/invoices bigquery://acme/bronze/fresh pubsub://acme/clicks " +
		"bigquery://acme/analysis/profile gs://lake/raw/manual/"
	if got := targetsOf(v); got != want {
		t.Errorf("rows =\n %s\nwant\n %s", got, want)
	}
	if v.Total != 6 || v.Shown != 6 {
		t.Errorf("total %d, shown %d, want 6 and 6", v.Total, v.Shown)
	}
	if !strings.Contains(targetsOf(BuildCatalog(catalogFixture(), DataFilter{Legacy: true}, catalogNow)), "bronze.payments") {
		t.Error("asking for legacy rows does not show them")
	}
}

// THE BAND COUNTS WHAT EACH CHOICE WOULD SHOW, attention being stale and late
// together, with the search and the kind as they are.
func TestTheCatalogsBandCountsEachStatus(t *testing.T) {
	v := BuildCatalog(catalogFixture(), DataFilter{}, catalogNow)
	for status, want := range map[string]int{
		"": 6, "attention": 2, "stale": 1, "late": 1, "on_time": 1, "continuous": 1, "paused": 1, "unscheduled": 1, "unidentified": 0,
	} {
		if got := v.Count(status); got != want {
			t.Errorf("%q counts %d, want %d", status, got, want)
		}
	}
	if got := BuildCatalog(catalogFixture(), DataFilter{Kind: "bigquery"}, catalogNow).Count("attention"); got != 1 {
		t.Errorf("attention among BigQuery counts %d, want 1", got)
	}
}

// THE FILTERS NARROW THE ROWS: attention, one status, a kind, and a search in
// the table, the dataset, the workflow or the gateway's path.
func TestTheCatalogFilters(t *testing.T) {
	for _, c := range []struct {
		query, want string
	}{
		{"status=attention", "bigquery://acme/bronze/old postgres://db/public/invoices"},
		{"status=continuous", "pubsub://acme/clicks"},
		{"kind=postgres", "postgres://db/public/invoices"},
		{"q=billing", "postgres://db/public/invoices"},
		{"q=analysis", "bigquery://acme/analysis/profile"},
		{"q=/v1/clicks", "pubsub://acme/clicks"},
		{"q=FRESH", "bigquery://acme/bronze/fresh"},
		{"q=payments&legacy=1", "bronze.payments"},
	} {
		q, _ := url.ParseQuery(c.query)
		if got := targetsOf(BuildCatalog(catalogFixture(), ParseDataFilter(q), catalogNow)); got != c.want {
			t.Errorf("%s draws %q, want %q", c.query, got, c.want)
		}
	}
}

// A ROW SAYS WHO WRITES IT: the first writer, and how many more.
func TestACatalogRowSaysWhoWritesIt(t *testing.T) {
	two := DataRow{Writers: []WriterView{
		{CatalogWriter: postgres.CatalogWriter{Workflow: "fire", Node: "fetch"}},
		{CatalogWriter: postgres.CatalogWriter{Workflow: "fire", Node: "backfill"}},
	}}
	if got, more := writerLine(two); got != "fire · fetch" || more != 1 {
		t.Errorf("writer line = %q +%d", got, more)
	}
	gw := DataRow{Writers: []WriterView{{CatalogWriter: postgres.CatalogWriter{Gateway: &postgres.GatewayWriter{Name: "edge", Stream: "/v1/clicks"}}}}}
	if got, _ := writerLine(gw); got != "gateway edge · /v1/clicks" {
		t.Errorf("gateway writer line = %q", got)
	}
}

// A DESTINATION'S ROW DIMS WHERE IT LIVES and keeps the table bright.
func TestADestinationIsDrawnUpToItsTable(t *testing.T) {
	for target, want := range map[string][2]string{
		"bigquery://acme/bronze/orders": {"bigquery://acme/bronze/", "orders"},
		"bigquery://acme/bronze/*":      {"bigquery://acme/bronze/", "*"},
		"gs://lake/raw/manual/":         {"gs://lake/raw/manual/", ""},
		"bronze.payments":               {"bronze.", "payments"},
	} {
		if where, what := splitDestination(target); where != want[0] || what != want[1] {
			t.Errorf("%s splits as %q %q", target, where, what)
		}
	}
}

// A WRITER'S REASON IS IN THE READER'S LANGUAGE (screen-catalog T3), for each
// status that has one; on time and continuous need none.
func TestAWritersReasonSpeaksEachLanguage(t *testing.T) {
	missed := WriterView{CatalogWriter: postgres.CatalogWriter{Workflow: "vendors", Node: "fetch", HasSchedule: true, Timezone: "UTC"},
		Verdict: catalog.Verdict{Status: catalog.Stale, Missed: clock(9, 0), Grace: 15 * time.Minute}}
	paused := WriterView{Verdict: catalog.Verdict{Status: catalog.Paused}}
	manual := WriterView{Verdict: catalog.Verdict{Status: catalog.Unscheduled}}
	unreadable := WriterView{CatalogWriter: postgres.CatalogWriter{HasSchedule: true}, Verdict: catalog.Verdict{Status: catalog.Unscheduled}}
	gateway := WriterView{CatalogWriter: postgres.CatalogWriter{Gateway: &postgres.GatewayWriter{Name: "edge", Stream: "/v1/clicks", Role: "sink", PublishedAt: clock(8, 0)}},
		Verdict: catalog.Verdict{Status: catalog.Continuous}}
	for lang, want := range map[string][5]string{
		"en":    {"vendors missed its", "Its schedule is paused", "It runs by hand", "cannot be read", "Published from gateway edge"},
		"pt-BR": {"vendors perdeu a execução", "A agenda está pausada", "Roda à mão", "não pode ser lida", "Publicado pelo gateway edge"},
		"es":    {"vendors perdió su ejecución", "Su programación está pausada", "Se ejecuta a mano", "no se puede leer", "Publicado por el gateway edge"},
	} {
		ctx := i18n.WithLang(context.Background(), lang)
		for i, w := range []WriterView{missed, paused, manual, unreadable, gateway} {
			if got := w.Reason(ctx); !strings.Contains(got, want[i]) {
				t.Errorf("%s: reason %d = %q, want %q in it", lang, i, got, want[i])
			}
		}
		if got := (WriterView{Verdict: catalog.Verdict{Status: catalog.OnTime}}).Reason(ctx); got != "" {
			t.Errorf("%s: on time has a reason: %q", lang, got)
		}
	}
}
