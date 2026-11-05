package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

type catalogFake struct {
	entries []postgres.CatalogEntry
	err     error

	detail *postgres.TargetDetail
	asked  *string

	streams map[postgres.StreamKey]*postgres.WriterConfig
	steps   map[postgres.StepKey]*postgres.WriterConfig
}

func (c catalogFake) StepConfigs(context.Context) (map[postgres.StepKey]*postgres.WriterConfig, error) {
	return c.steps, nil
}

func (c catalogFake) StreamConfigs(context.Context) (map[postgres.StreamKey]*postgres.WriterConfig, error) {
	return c.streams, nil
}

func (c catalogFake) Catalog(context.Context) ([]postgres.CatalogEntry, error) {
	return c.entries, c.err
}

func (c catalogFake) CatalogTarget(_ context.Context, target string) (*postgres.TargetDetail, error) {
	if c.asked != nil {
		*c.asked = target
	}
	if c.detail == nil || c.detail.Target != target {
		return nil, nil
	}
	return c.detail, nil
}

func dataUI(c api.CatalogReader) *api.UI {
	return api.NewUI(nil, nil, nil, nil, nil, c, branding.Default(), slog.New(slog.DiscardHandler))
}

func get(t *testing.T, ui *api.UI, path string) (int, string) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(body)
}

func i64(v int64) *int64 { return &v }

func demoCatalog() []postgres.CatalogEntry {
	now := time.Now()
	return []postgres.CatalogEntry{
		{
			Target: "postgres://brevis_it/public/landing_orders", Kind: "postgres",
			Writers: []postgres.CatalogWriter{{Workflow: "sdk_orders", Node: "load",
				LastLoaded: now.Add(-time.Minute), LastRows: i64(37),
				HasSchedule: true, Cron: "* * * * *", Timezone: "UTC", Active: true}},
			Recent: []*int64{i64(0), i64(37)},
		},
		{
			Target: "s3://demo-landing/reports/", Kind: "s3",
			Writers: []postgres.CatalogWriter{{Workflow: "shell_report", Node: "build_report",
				LastLoaded: now.Add(-2 * time.Minute)}},
			Recent: []*int64{nil},
		},
		{
			Target: "bronze.payments", Legacy: true,
			Writers: []postgres.CatalogWriter{{Workflow: "payments", Node: "load", LastLoaded: now.Add(-48 * time.Hour), LastRows: i64(10)}},
		},
	}
}

// Catalog's tests, restated on the screens that inherited each behavior
// (IN6): the list's on /ingest/loads, the destination page's on a load's page,
// the gateway's on /ingest/streams.

func gatewayCatalog() []postgres.CatalogEntry {
	gw := func(stream, role, note string) postgres.CatalogWriter {
		return postgres.CatalogWriter{Gateway: &postgres.GatewayWriter{Name: "edge", Stream: stream,
			Role: role, Kind: "files", Note: note, PublishedAt: time.Now().Add(-time.Hour)}}
	}
	return []postgres.CatalogEntry{
		{Target: "pubsub://brevis-local/clicks", Kind: "pubsub", Writers: []postgres.CatalogWriter{gw("/v1/clicks", "sink", "")}},
		{Writers: []postgres.CatalogWriter{gw("/v1/clicks", "dead_letter", `"./dead-letter/" is relative to the gateway's working directory`)}},
	}
}
