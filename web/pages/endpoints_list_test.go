package pages

import (
	"strings"
	"testing"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// endpointsFixture is three endpoints over the catalog fixture (a fixed
// 11:20): one reading a table on time, one a stale one, one a table the
// catalog does not know; the first two from an API with a Redis cache, the
// last from one published before its manifest was kept.
func endpointsFixture() ([]postgres.PublishedEndpoint, []postgres.CatalogEntry) {
	redis := postgres.APIManifest{Connection: "warehouse", Dialect: "bigquery",
		Cache: &catalog.EndpointsCache{Type: "redis", AddrFrom: "BREVIS_CACHE_ADDR", TTL: "15m"}}
	ep := func(api, name, path, about string, reads []string, m postgres.APIManifest, params ...catalog.EndpointParam) postgres.PublishedEndpoint {
		e := catalog.Endpoint{Name: name, Path: path, Tenant: "key", Reads: reads, Params: params,
			Notes: map[string]string{"": about}}
		return postgres.PublishedEndpoint{API: api, Name: name, Path: path, Tenant: "key", Reads: reads,
			Endpoint: e, PublishedAt: clock(9, 20), Manifest: m}
	}
	all := []postgres.PublishedEndpoint{
		ep("metrics", "fresh", "/v1/chart/fresh", "Fresh orders per hour.\nMore detail here.", []string{"bigquery://acme/bronze/fresh"}, redis,
			catalog.EndpointParam{Name: "grain", Type: "string", Required: true}, catalog.EndpointParam{Name: "day", Type: "date"}),
		ep("metrics", "old", "/v1/kpi/old", "Vendors, stale.", []string{"bigquery://acme/bronze/old", "bigquery://acme/bronze/fresh"}, redis),
		ep("legacy", "lost", "/v1/lost", "", []string{"postgres://nowhere/public/x"}, postgres.APIManifest{}),
	}
	return all, catalogFixture()
}

func endpointKeys(v EndpointsView) string {
	var out []string
	for _, r := range v.Rows {
		out = append(out, r.Name)
	}
	return strings.Join(out, " ")
}

// THE ENDPOINTS ARE WORST FIRST BY THE DATA THEY READ (screen-endpoints T2):
// an endpoint is as stale as the worst table it reads; one reading only
// tables the catalog does not know has no freshness to say.
func TestTheEndpointsAreWorstFirstByTheDataTheyRead(t *testing.T) {
	all, entries := endpointsFixture()
	v := BuildEndpoints(all, entries, EndpointFilter{}, catalogNow)
	if got := endpointKeys(v); got != "old fresh lost" {
		t.Errorf("rows = %s", got)
	}
	states := map[string]string{}
	for _, r := range v.Rows {
		states[r.Name] = r.Freshness
	}
	if states["old"] != "stale" || states["fresh"] != "fresh" || states["lost"] != "" {
		t.Errorf("states = %v", states)
	}
	if v.Total != 3 || v.APIs != 2 || v.Fresh != 1 || v.Stale != 1 {
		t.Errorf("total %d, apis %d, fresh %d, stale %d", v.Total, v.APIs, v.Fresh, v.Stale)
	}
}

// THE LIST FILTERS BY API AND BY STATE (screen-endpoints T3), in the URL:
// the APIs offered are the ones present, and the counts are every
// endpoint's whatever is shown.
func TestTheEndpointsFilterByAPIAndState(t *testing.T) {
	all, entries := endpointsFixture()
	for _, c := range []struct {
		f    EndpointFilter
		want string
	}{
		{EndpointFilter{API: "metrics"}, "old fresh"},
		{EndpointFilter{API: "legacy"}, "lost"},
		{EndpointFilter{State: "stale"}, "old"},
		{EndpointFilter{State: "fresh"}, "fresh"},
		{EndpointFilter{API: "metrics", State: "fresh", Q: "chart"}, "fresh"},
		{EndpointFilter{API: "legacy", State: "fresh"}, ""},
	} {
		v := BuildEndpoints(all, entries, c.f, catalogNow)
		if got := endpointKeys(v); got != c.want {
			t.Errorf("%+v: rows = %q, want %q", c.f, got, c.want)
		}
		if v.Total != 3 {
			t.Errorf("%+v: the total counts what is shown (%d)", c.f, v.Total)
		}
	}
	v := BuildEndpoints(all, entries, EndpointFilter{}, catalogNow)
	if strings.Join(v.APINames, " ") != "legacy metrics" {
		t.Errorf("the APIs offered = %v", v.APINames)
	}
}
