package pages

import (
	"testing"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// endpointPageFixture is the endpoints fixture with the fresh table's writer
// describing where it reads, and one endpoint reading a gateway's sink.
func endpointPageFixture() ([]postgres.PublishedEndpoint, []postgres.CatalogEntry) {
	all, entries := endpointsFixture()
	entries[0].Writers[0].Config = &postgres.WriterConfig{Fields: []catalog.ManifestField{
		{Path: "source.driver", Value: "from.HTTP"}, {Path: "source.url", Value: "https://api.example.com/v1/orders"},
	}}
	clicks := catalog.Endpoint{Name: "clicks", Path: "/v1/clicks", Tenant: "none", Reads: []string{"pubsub://acme/clicks"}}
	all = append(all, postgres.PublishedEndpoint{API: "metrics", Name: "clicks", Path: "/v1/clicks", Tenant: "none",
		Reads: clicks.Reads, Endpoint: clicks, PublishedAt: clock(9, 20), Manifest: all[0].Manifest})
	return all, entries
}

func endpointNamed(all []postgres.PublishedEndpoint, name string) postgres.PublishedEndpoint {
	for _, e := range all {
		if e.Name == name {
			return e
		}
	}
	panic(name)
}

// WHERE THE DATA COMES FROM IS ONE LINE PER TABLE READ (screen-endpoints
// T4): the writer's declared origin, the writer -- a load or a stream, each
// a link -- and the table with how it stands. A table nobody known writes is
// the table alone.
func TestAnEndpointsLineageIsOneLinePerTable(t *testing.T) {
	all, entries := endpointPageFixture()
	v := BuildEndpoint(endpointNamed(all, "old"), entries, catalogNow)
	if len(v.Lineage) != 2 || v.Lineage[0].Target != "bigquery://acme/bronze/old" || v.Lineage[1].Target != "bigquery://acme/bronze/fresh" {
		t.Fatalf("lineage = %+v", v.Lineage)
	}
	old, fresh := v.Lineage[0], v.Lineage[1]
	if old.Writer == nil || old.WriterHref() != "/ingest/loads/vendors/fetch" || !old.Stale() || old.Source.Kind != "" {
		t.Errorf("the stale table's line: %+v", old)
	}
	if fresh.Stale() || fresh.Source.Kind != "http" || fresh.Source.Where != "https://api.example.com/v1/orders" {
		t.Errorf("the fresh table's line: %+v", fresh)
	}
	if v.Row.Freshness != "stale" {
		t.Errorf("the endpoint stands %q", v.Row.Freshness)
	}

	clicks := BuildEndpoint(endpointNamed(all, "clicks"), entries, catalogNow).Lineage[0]
	if clicks.WriterHref() != "/ingest/streams/edge/v1/clicks" || clicks.Writer.Gateway == nil {
		t.Errorf("a stream's line: %+v", clicks)
	}
	lost := BuildEndpoint(endpointNamed(all, "lost"), entries, catalogNow)
	if len(lost.Lineage) != 1 || lost.Lineage[0].Known || lost.Lineage[0].Writer != nil || lost.Row.Freshness != "" {
		t.Errorf("a table nobody known writes: %+v", lost.Lineage)
	}
}

// CLEAR CACHE IS A COMMAND TO COPY (D5), built from what the API published:
// the variable its cache's address is read from, its dialect, and the tables
// read as a build keys them -- with no project on BigQuery, lowered on
// Postgres. A table of another warehouse is not the cache's; with no cache
// on record there is no command.
func TestTheClearCacheCommandIsBuiltFromWhatWasPublished(t *testing.T) {
	all, entries := endpointPageFixture()
	if got := BuildEndpoint(endpointNamed(all, "old"), entries, catalogNow).BumpCommand(); got != "brevis-sql generation bump --generations-from BREVIS_CACHE_ADDR --dialect bigquery bronze.old bronze.fresh" {
		t.Errorf("bigquery: %s", got)
	}
	pg := postgres.PublishedEndpoint{API: "sales", Name: "orders", Path: "/v1/orders",
		Reads: []string{"postgres://db/Public/Invoices", "gs://lake/raw/manual/"},
		Manifest: postgres.APIManifest{Connection: "pg", Dialect: "postgres",
			Cache: &catalog.EndpointsCache{Type: "redis", AddrFrom: "CACHE_ADDR", TTL: "1h"}}}
	if got := BuildEndpoint(pg, nil, catalogNow).BumpCommand(); got != "brevis-sql generation bump --generations-from CACHE_ADDR --dialect postgres public.invoices" {
		t.Errorf("postgres: %s", got)
	}
	if got := BuildEndpoint(endpointNamed(all, "lost"), entries, catalogNow).BumpCommand(); got != "" {
		t.Errorf("no cache on record: %s", got)
	}
}
