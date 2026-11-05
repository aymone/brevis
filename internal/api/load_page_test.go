package api_test

import (
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// loadCatalog is daily_sales' report landing on a file, a second workflow
// writing the same file (whose loads must not leak into report's page), and
// a step that writes two destinations.
func loadCatalog() catalogFake {
	now := time.Now()
	report := postgres.CatalogWriter{Workflow: "daily_sales", Node: "report", LastLoaded: now.Add(-47 * time.Minute),
		LastRows: i64(4), HasSchedule: true, Cron: "0 4 * * *", Timezone: "UTC", Active: true}
	other := postgres.CatalogWriter{Workflow: "backfill", Node: "dump", LastLoaded: now.Add(-time.Hour), LastRows: i64(9)}
	entry := postgres.CatalogEntry{Target: "file:///data/reports/", Kind: "file", Writers: []postgres.CatalogWriter{report, other}}
	return catalogFake{
		entries: []postgres.CatalogEntry{
			entry,
			{Target: "s3://lake/a/", Kind: "s3", Writers: []postgres.CatalogWriter{{Workflow: "multi", Node: "split", LastLoaded: now}}},
			{Target: "s3://lake/b/", Kind: "s3", Writers: []postgres.CatalogWriter{{Workflow: "multi", Node: "split", LastLoaded: now}}},
		},
		detail: &postgres.TargetDetail{CatalogEntry: entry, Loads: []postgres.TargetLoad{
			{RunID: "8d36ef6e-0000-0000-0000-000000000001", Workflow: "daily_sales", Node: "report", LoadedAt: now.Add(-47 * time.Minute), Rows: i64(4), Bytes: i64(149)},
			{RunID: "77777777-0000-0000-0000-000000000009", Workflow: "backfill", Node: "dump", LoadedAt: now.Add(-time.Hour), Rows: i64(9)},
			{RunID: "6420b0fd-0000-0000-0000-000000000002", Workflow: "daily_sales", Node: "report", LoadedAt: now.Add(-25 * time.Hour), Rows: i64(0)},
		}},
	}
}
