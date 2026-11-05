package pages

import (
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func gw(name, stream, role string, fields ...catalog.ManifestField) postgres.CatalogWriter {
	w := &postgres.GatewayWriter{Name: name, Stream: stream, Role: role}
	if len(fields) > 0 {
		w.Config = &postgres.WriterConfig{Fields: fields}
	}
	return postgres.CatalogWriter{Gateway: w}
}

func step(workflow, node, cron string, last time.Time, rows int64) postgres.CatalogWriter {
	return postgres.CatalogWriter{Workflow: workflow, Node: node, LastLoaded: last, LastRows: &rows,
		HasSchedule: cron != "", Cron: cron, Timezone: "UTC", Active: cron != ""}
}

// printFixtures are the catalog as the prints show it: events_gateway's two
// streams, each with a sink and a dead letter; a third stream with no dead
// letter; daily_sales's three steps; a legacy label; and a step that writes
// two destinations.
func printFixtures(now time.Time) []postgres.CatalogEntry {
	f := func(path string, v any) catalog.ManifestField { return catalog.ManifestField{Path: path, Value: v} }
	ago := now.Add(-47 * time.Minute)
	return []postgres.CatalogEntry{
		{Target: "pubsub://brevis-local/clicks", Kind: "pubsub", Writers: []postgres.CatalogWriter{
			gw("events_gateway", "/v1/clicks", "sink", f("name", "clicks"), f("path", "/v1/clicks"),
				f("format", "json"), f("hook", "enrich_clicks"), f("buffer.flush.every", "1s"),
				f("buffer.flush.records", 500), f("sink.type", "pubsub"))}},
		{Target: "file:///var/dead-letter/clicks/", Kind: "file", Writers: []postgres.CatalogWriter{
			gw("events_gateway", "/v1/clicks", "dead_letter")}},
		{Target: "postgres://warehouse/sales/events", Kind: "postgres", Writers: []postgres.CatalogWriter{
			gw("events_gateway", "/v1/orders", "sink", f("path", "/v1/orders"), f("format", "json"),
				f("sink.type", "postgres"), f("sink.write", "merge"))}},
		{Target: "file:///var/dead-letter/orders/", Kind: "file", Writers: []postgres.CatalogWriter{
			gw("events_gateway", "/v1/orders", "dead_letter")}},
		{Target: "bigquery://zarv-development-94b6/bronze/*", Kind: "bigquery", Writers: []postgres.CatalogWriter{
			gw("zarv_ingestion_dev", "/v1/ingestion", "sink")}},
		{Target: "file:///data/reports/2026-10-15/", Kind: "file", Writers: []postgres.CatalogWriter{
			step("daily_sales", "report", "0 4 * * *", ago, 4)}},
		{Target: "postgres://warehouse/sales/daily", Kind: "postgres", Writers: []postgres.CatalogWriter{
			step("daily_sales", "load_warehouse", "0 4 * * *", ago, 2)}},
		{Target: "file:///data/landing/", Kind: "file", Writers: []postgres.CatalogWriter{
			step("daily_sales", "load", "0 4 * * *", ago, 2)}},
		{Target: "bronze.vendors_bacen_sgs", Legacy: true, Writers: []postgres.CatalogWriter{
			step("vendors_bacen", "fetch_series", "", now.Add(-15*24*time.Hour), 0)}},
		{Target: "s3://lake/a/", Kind: "s3", Writers: []postgres.CatalogWriter{step("multi", "split", "", ago, 1)}},
		{Target: "s3://lake/b/", Kind: "s3", Writers: []postgres.CatalogWriter{step("multi", "split", "", ago, 1)}},
	}
}

// A STREAM IS THE UNIT: one sink and its dead letter, keyed by gateway and
// path. A dead letter joins its stream and is never an item of its own.
func TestIngestStreamsJoinEachDeadLetterToItsStream(t *testing.T) {
	gws := StreamsOf(printFixtures(time.Now()))
	if len(gws) != 2 || gws[0].Name != "events_gateway" || gws[1].Name != "zarv_ingestion_dev" {
		t.Fatalf("gateways = %+v", gws)
	}
	ev := gws[0].Streams
	if len(ev) != 2 {
		t.Fatalf("events_gateway has %d streams, want 2 (a dead letter became a stream?)", len(ev))
	}
	clicks := ev[0]
	if clicks.Path != "/v1/clicks" || clicks.Sink.URI != "pubsub://brevis-local/clicks" || clicks.Sink.Kind != "pubsub" {
		t.Errorf("clicks = %+v", clicks)
	}
	if clicks.DeadLetter == nil || clicks.DeadLetter.URI != "file:///var/dead-letter/clicks/" || clicks.DeadLetter.Kind != "file" {
		t.Errorf("clicks' dead letter = %+v", clicks.DeadLetter)
	}
	if clicks.Field("hook") != "enrich_clicks" || clicks.Field("format") != "json" {
		t.Error("the stream does not read its declared fields")
	}
	if ev[1].Path != "/v1/orders" || ev[1].DeadLetter == nil || ev[1].Merge() == nil || !*ev[1].Merge() {
		t.Errorf("orders = %+v (merge is sink.write == merge)", ev[1])
	}
	if clicks.Merge() != nil {
		t.Error("a stream that does not declare its write mode is called merge or not")
	}
	ing := gws[1].Streams
	if len(ing) != 1 || ing[0].DeadLetter != nil || ing[0].Config != nil {
		t.Errorf("a stream with no dead letter and no config = %+v", ing)
	}
	for _, g := range gws {
		for _, s := range g.Streams {
			if strings.Contains(s.Sink.URI, "dead-letter") {
				t.Errorf("a dead letter is a stream's sink: %+v", s)
			}
		}
	}
}

// A LOAD'S LINK NAMES ITS DESTINATION only when its step writes more than one.
func TestIngestHrefs(t *testing.T) {
	now := time.Now()
	loads := map[string]LoadTarget{}
	for _, l := range LoadsOf(printFixtures(now), now) {
		loads[l.Step+" "+l.URI] = l
	}
	if h := loads["report file:///data/reports/2026-10-15/"].Href(); h != "/ingest/loads/daily_sales/report" {
		t.Errorf("a one-destination step's link = %q", h)
	}
	if h := loads["split s3://lake/a/"].Href(); !strings.Contains(h, "?target=s3%3A%2F%2Flake%2Fa%2F") {
		t.Errorf("a two-destination step's link = %q", h)
	}
	s := StreamsOf(printFixtures(now))[0].Streams[0]
	if s.Href() != "/ingest/streams/events_gateway/v1/clicks" {
		t.Errorf("a stream's link = %q", s.Href())
	}
}

// QUERY DESTINATION ONLY WHERE SQL CAN READ IT: not Pub/Sub, not a file.
func TestAStreamQueriesOnlyATable(t *testing.T) {
	gws := StreamsOf(printFixtures(time.Now()))
	if BuildStream(gws[0].Streams[0], nil).Queryable() {
		t.Error("a Pub/Sub sink offers a query")
	}
	if !BuildStream(gws[0].Streams[1], nil).Queryable() {
		t.Error("a Postgres sink offers no query")
	}
	// An Iceberg table is a table, and SQL will read it one day -- but not
	// through this workbench yet: brevis-sql speaks Postgres and BigQuery,
	// and a query link that opens a page unable to answer is worse than none.
	iceberg := Stream{Gateway: "edge", Path: "/v1/events", Sink: Place{Kind: "iceberg", URI: "iceberg://lake/bronze/events"}}
	if BuildStream(iceberg, nil).Queryable() {
		t.Error("an Iceberg sink offers a query the workbench cannot answer yet")
	}
}

// THE EXAMPLE IS IN THE STREAM'S FORMAT: a list for `array`, one object per
// line for `ndjson`, and the source key an identity declares filled in.
func TestTheTestEventExampleFollowsTheFormat(t *testing.T) {
	for _, c := range []struct{ format, key, want string }{
		{"array", "", `[{"event": "test"}]`},
		{"ndjson", "", "{\"event\": \"test\"}\n{\"event\": \"test\"}"},
		{"json", "table_name", `{"event": "test", "table_name": "test"}`},
	} {
		f := []catalog.ManifestField{{Path: "format", Value: c.format}}
		if c.key != "" {
			f = append(f, catalog.ManifestField{Path: "identity.source_key", Value: c.key})
		}
		v := StreamView{Stream: Stream{Path: "/v1/x", Config: &postgres.WriterConfig{Fields: f}}}
		if got := v.TestBody(); got != c.want {
			t.Errorf("%s example = %q, want %q", c.format, got, c.want)
		}
	}
}
