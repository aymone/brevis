package pages

import (
	"encoding/json"
	"testing"
	"time"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// richPipeline is an SDK pipeline that fills every part of the drawer's
// configuration: three sources, a transformation and an aggregation, a
// declared schema, a merge, and a change in its history.
func richPipeline() *postgres.WriterConfig {
	f := func(p string, v any) catalog.ManifestField { return catalog.ManifestField{Path: p, Value: v} }
	fields := []catalog.ManifestField{
		f("source.driver", "from.Many"), f("source.workers", 2.0),
		f("source.sources.1.driver", "from.Files"), f("source.sources.1.path", "/in/a.csv"),
		f("source.sources.2.driver", "from.Files"), f("source.sources.2.path", "/in/b.csv"),
		f("source.sources.3.driver", "from.HTTP"), f("source.sources.3.url", "https://api.acme.io"),
		f("transform.1.map", []any{"compute(loaded_by)"}),
		f("transform.2.aggregate.by", []any{"sku"}), f("transform.2.aggregate.qty", "sum(qty)"),
		f("target.driver", "bigquery.Table"), f("target.project", "acme"), f("target.dataset", "bronze"), f("target.table", "orders"),
		f("target.declared.columns", []any{"sku", "qty"}), f("target.declared.schema", []any{"sku STRING", "qty INT64"}),
		f("target.dedup", "merge"),
	}
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	old := append([]catalog.ManifestField{}, fields[:len(fields)-1]...)
	old = append(old, f("target.dedup", "none"), f("target.timeout", "30s"))
	return &postgres.WriterConfig{Fields: fields, History: []postgres.ConfigVersion{
		{Fields: fields, At: at, RunID: "aaaaaaaa-0000-0000-0000-000000000001"},
		{Fields: old, At: at.Add(-time.Hour), RunID: "aaaaaaaa-0000-0000-0000-000000000002"},
	}}
}

// THE SUMMARY IS SENT AS KEYS AND DATA, never as a sentence: the server has no language. The app puts the pieces
// together and says them in the reader's language; saidEN reads them the way the app does in English.
func TestTheSummaryIsSentAsKeysAndData(t *testing.T) {
	stream := &postgres.WriterConfig{Fields: []catalog.ManifestField{
		{Path: "format", Value: "json"}, {Path: "path", Value: "/v1/clicks"},
		{Path: "sink.type", Value: "pubsub"}, {Path: "sink.project", Value: "acme"}, {Path: "sink.topic", Value: "clicks"},
		{Path: "sink.write", Value: "merge"}, {Path: "buffer.flush.every", Value: "1m"}, {Path: "buffer.flush.records", Value: 2000.0},
		{Path: "dead_letter.type", Value: "files"}, {Path: "dead_letter.path", Value: "./dl/"},
	}}
	raw, err := json.Marshal(Summary(stream))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"$":"cat","v":[{"k":"summary.intake.json"}," ",{"k":"summary.stream.posted","a":{"path":"/v1/clicks"}},` +
		`" ",{"k":"summary.stream.published","a":{"to":{"k":"summary.dest.topic","a":{"topic":"acme/clicks"}}}},` +
		`", ",{"k":"summary.stream.merge"},", ",{"k":"summary.stream.batch.both","a":{"every":{"k":"summary.unit.minute","n":1},` +
		`"records":{"k":"summary.events","n":2000,"a":{"n":{"$":"num","v":2000}}}}},` +
		`"; ",{"k":"summary.stream.refused","a":{"to":{"k":"summary.dest.files","a":{"path":"./dl/"}}}},"."]}`
	if string(raw) != want {
		t.Errorf("stream summary:\n got %s\nwant %s", raw, want)
	}
	if got := saidEN(Summary(richPipeline())); got != "Reads 3 sources, applies 1 transformation, groups by sku and writes to BigQuery table acme.bronze.orders, merged by ingestion id so a re-run changes nothing." {
		t.Errorf("pipeline summary = %s", got)
	}
}
