package api_test

import (
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// catalogEntries are destinations whose status does not hang on the minute:
// an hourly writer silent for five hours is stale whatever the clock says.
func catalogEntries() []postgres.CatalogEntry {
	now := time.Now()
	return []postgres.CatalogEntry{
		{Target: "bigquery://acme/bronze/old", Kind: "bigquery", Writers: []postgres.CatalogWriter{{Workflow: "vendors", Node: "fetch",
			LastLoaded: now.Add(-5 * time.Hour), LastRows: i64(12), HasSchedule: true, Cron: "0 * * * *", Timezone: "UTC", Active: true}},
			Recent: []*int64{i64(10), nil, i64(12)}},
		{Target: "pubsub://acme/clicks", Kind: "pubsub", Writers: []postgres.CatalogWriter{{
			Gateway: &postgres.GatewayWriter{Name: "edge", Stream: "/v1/clicks", Role: "sink"}, LastLoaded: now.Add(-time.Minute)}}},
		{Target: "gs://lake/raw/manual/", Kind: "gs", Writers: []postgres.CatalogWriter{{Workflow: "export", Node: "write", LastLoaded: now.Add(-2 * time.Hour)}}},
		{Target: "bronze.payments", Legacy: true, Writers: []postgres.CatalogWriter{{Workflow: "payments", Node: "load", LastLoaded: now.Add(-48 * time.Hour)}}},
	}
}

// THE PALETTE FINDS A DESTINATION, with who writes it, and opens it.
func TestThePaletteFindsADestination(t *testing.T) {
	_, _, items := palette(t, dataUI(catalogFake{entries: catalogEntries()}))
	for _, it := range items {
		if it.Group == "Catalog" && said(it.Label) == "bigquery://acme/bronze/old" {
			if it.Hint != "vendors · fetch" || it.Href != "/data/target?u=bigquery%3A%2F%2Facme%2Fbronze%2Fold" {
				t.Errorf("the destination's entry = %+v", it)
			}
			return
		}
	}
	t.Error("the palette has no destination")
}

// drawerUI is the catalog with one destination's detail: the stale table,
// its loads, and an endpoint that reads it.
func drawerUI() *api.UI {
	entries := catalogEntries()
	now := time.Now()
	detail := &postgres.TargetDetail{CatalogEntry: entries[0], Loads: []postgres.TargetLoad{
		{RunID: "aaaaaaaa-0000-0000-0000-000000000001", Workflow: "vendors", Node: "fetch", LoadedAt: now.Add(-6 * time.Hour), Rows: i64(10)},
		{RunID: "aaaaaaaa-0000-0000-0000-000000000002", Workflow: "vendors", Node: "fetch", LoadedAt: now.Add(-5 * time.Hour), Rows: i64(12)},
	}}
	reads := endpointsFake{all: []postgres.PublishedEndpoint{{API: "metrics-api", Name: "old", Path: "/v1/old",
		Reads: []string{"bigquery://acme/bronze/old"}, PublishedAt: now}}}
	return dataUI(catalogFake{entries: entries, detail: detail}).WithEndpoints(reads)
}
