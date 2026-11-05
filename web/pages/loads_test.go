package pages

import (
	"net/url"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// loadsFixture is the catalog fixture (a destination in every status at a
// fixed 11:20) with the fresh step's configuration: an HTTP source.
func loadsFixture() []postgres.CatalogEntry {
	entries := catalogFixture()
	AttachStepConfigs(entries, map[postgres.StepKey]*postgres.WriterConfig{
		{Workflow: "orders", Node: "load", Target: "bigquery://acme/bronze/fresh"}: sourceConfig(
			field("source.driver", "from.HTTP"), field("source.url", "https://api.acme.io/orders")),
	})
	return entries
}

func loadKeys(v LoadsView) string {
	var out []string
	for _, r := range v.Rows {
		out = append(out, r.Workflow+"·"+r.Step)
	}
	return strings.Join(out, " ")
}

// THE LOADS ARE ONE ROW PER STEP AND DESTINATION, WORST FIRST (screen-loads
// T2): the Catalog's order -- stale, late, on time, paused, no schedule --
// and within a status the most recently loaded first. A gateway's stream is
// not a load, and a legacy label waits until it is asked for, as in the
// Catalog (T3).
func TestTheLoadsAreWorstFirst(t *testing.T) {
	v := BuildLoads(loadsFixture(), DataFilter{}, catalogNow)
	if got, want := loadKeys(v), "vendors·fetch billing·sync orders·load profile·run export·write"; got != want {
		t.Errorf("rows =\n %s\nwant\n %s", got, want)
	}
	if v.Total != 5 || v.Workflows != 5 {
		t.Errorf("total %d, workflows %d, want 5 and 5", v.Total, v.Workflows)
	}
	if got := loadKeys(BuildLoads(loadsFixture(), DataFilter{Legacy: true}, catalogNow)); !strings.HasSuffix(got, "payments·load") {
		t.Errorf("asking for legacy rows does not show them: %s", got)
	}
}

// THE FILTERS NARROW THE ROWS (screen-loads T3): attention, one status, the
// origin -- declared or not -- and a search in the workflow, the step, the
// origin and the destination.
func TestTheLoadsFilter(t *testing.T) {
	for query, want := range map[string]string{
		"status=attention":          "vendors·fetch billing·sync",
		"status=paused":             "profile·run",
		"source=http":               "orders·load",
		"source=none":               "vendors·fetch billing·sync profile·run export·write",
		"source=none&status=late":   "billing·sync",
		"q=BILLING":                 "billing·sync",
		"q=fetch":                   "vendors·fetch",
		"q=api.acme.io":             "orders·load",
		"q=analysis":                "profile·run",
		"q=payments&legacy=1":       "payments·load",
		"source=bogus&status=bogus": "vendors·fetch billing·sync orders·load profile·run export·write",
	} {
		q, _ := url.ParseQuery(query)
		if got := loadKeys(BuildLoads(loadsFixture(), ParseDataFilter(q), catalogNow)); got != want {
			t.Errorf("%s draws %q, want %q", query, got, want)
		}
	}
}
