package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// readsFake answers the two reads the palette needs; anything else it is asked
// panics, which is also how a test learns the index reads more than it says.
type readsFake struct {
	api.Reads
	workflows []postgres.WorkflowSummary
	projects  []postgres.ProjectSummary

	// recent answers RecentStates, and recentCalls counts the calls: the
	// strip is one query per page, not one per row.
	recent      map[string][]postgres.RecentRun
	recentCalls *int
	// recentN is how many runs the strip asked for.
	recentN *int

	// connects answers Connects: the schemes each workflow wrote.
	connects map[string][]string
	// lastRuns and stepConnects answer the workflow graph's per-step reads.
	lastRuns     map[string]postgres.StepRun
	stepConnects map[string][]string
}

func (f readsFake) LastStepRuns(context.Context, string) (map[string]postgres.StepRun, error) {
	return f.lastRuns, nil
}

func (f readsFake) StepConnects(context.Context, string, time.Time) (map[string][]string, error) {
	return f.stepConnects, nil
}

// Schedules are the workflows' own, for the trigger's timezone.
func (f readsFake) Schedules(context.Context) ([]postgres.ScheduleSummary, error) {
	var out []postgres.ScheduleSummary
	for _, w := range f.workflows {
		if w.Cron != "" {
			out = append(out, postgres.ScheduleSummary{WorkflowSlug: w.Slug, Cron: w.Cron, Timezone: w.Timezone, Active: w.Active})
		}
	}
	return out, nil
}

func (f readsFake) Connects(context.Context, time.Time) (map[string][]string, error) {
	return f.connects, nil
}

func (f readsFake) RecentStates(_ context.Context, slugs []string, n int) (map[string][]postgres.RecentRun, error) {
	if f.recentCalls != nil {
		*f.recentCalls++
	}
	if f.recentN != nil {
		*f.recentN = n
	}
	out := map[string][]postgres.RecentRun{}
	for _, s := range slugs {
		if r, ok := f.recent[s]; ok {
			out[s] = r
		}
	}
	return out, nil
}

func (f readsFake) Workflows(context.Context) ([]postgres.WorkflowSummary, error) {
	return f.workflows, nil
}
func (f readsFake) Projects(context.Context) ([]postgres.ProjectSummary, error) {
	return f.projects, nil
}

// An empty Runs column, for the tests that open one run and read nothing of
// the list beside it.
func (f readsFake) Runs(context.Context, postgres.RunFilter) ([]postgres.RunSummary, error) {
	return nil, nil
}
func (f readsFake) CountRuns(context.Context, postgres.RunFilter) (int, error) { return 0, nil }
func (f readsFake) CountRunsInFlight(context.Context) (int, error)             { return 0, nil }
func (f readsFake) CountRunsByState(context.Context, postgres.RunFilter) (map[string]int, error) {
	return nil, nil
}

// paletteItem is what the index sends: a label and a hint are a name the data owns (a string) or a sentence the
// app says ({"k": catalog key, ...}); said is what the test compares, the string or the key.
type paletteItem struct {
	Group, Href, Icon, Action string
	Label, Hint               any
}

func said(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		return "{" + x["k"].(string) + "}"
	}
	return ""
}

func palette(t *testing.T, ui *api.UI) (int, http.Header, []paletteItem) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/palette", nil))
	var body struct{ Items []paletteItem }
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("the index is not JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec.Code, rec.Header(), body.Items
}

func paletteUI() *api.UI {
	reads := readsFake{
		workflows: []postgres.WorkflowSummary{{Slug: "daily_sales", Name: "daily_sales", Project: "analytics"}},
		projects:  []postgres.ProjectSummary{{Slug: "analytics", Name: "analytics", Workflows: 12}},
	}
	return api.NewUI(reads, nil, nil, nil, nil, catalogFake{entries: demoCatalog()}, branding.Default(), slog.New(slog.DiscardHandler)).
		WithEndpoints(published())
}

func find(items []paletteItem, group, label string) *paletteItem {
	for i := range items {
		if items[i].Group == group && said(items[i].Label) == label {
			return &items[i]
		}
	}
	return nil
}

// THE INDEX IS EVERYTHING THE CONSOLE LISTS, EACH WITH WHERE IT GOES.
//
// One JSON answer the palette fetches once and filters in the browser: the
// sections, then the workflows, projects, destinations and endpoints. The
// server says no words of its own: a sentence is a catalog key ({nav.loads})
// and the app builds what a search looks in from what it says.
func TestThePaletteIndexesWhatTheConsoleLists(t *testing.T) {
	code, hdr, items := palette(t, paletteUI())
	if code != http.StatusOK {
		t.Fatalf("/api/palette answered %d", code)
	}
	for _, want := range []struct{ group, label, href string }{
		{"Sections", "{nav.loads}", "/ingest/loads"},
		{"Workflows", "daily_sales", "/workflows/daily_sales"},
		{"Projects", "analytics", "/workflows?project=analytics"},
		{"Loads", "build_report → s3://demo-landing/reports/", "/ingest/loads/shell_report/build_report"},
		{"Endpoints", "/v1/clicks", "/data/endpoints/metrics-api/clicks"},
	} {
		it := find(items, want.group, want.label)
		if it == nil {
			t.Errorf("the index has no %s %q", want.group, want.label)
			continue
		}
		if it.Href != want.href {
			t.Errorf("%s %q goes to %q, want %q", want.group, want.label, it.Href, want.href)
		}
	}
	// It names what a login can see: kept by nobody between requests.
	if cc := hdr.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("the index is cacheable: %q", cc)
	}
}

// AND ONLY WHAT EXISTS HERE. The sidebar offers SQL only with a SQL service
// and the index agrees, because both read one list; a console with no
// endpoints reader lists no endpoints and no error.
func TestThePaletteOffersOnlyWhatThisConsoleHas(t *testing.T) {
	_, _, items := palette(t, dataUI(catalogFake{entries: demoCatalog()}))
	if find(items, "Sections", "{nav.sql}") != nil {
		t.Error("the index offers SQL on a console with no SQL service")
	}
	for _, it := range items {
		if it.Group == "Endpoints" || it.Group == "Workflows" {
			t.Errorf("the index lists %s %q from a reader that is not there", it.Group, it.Label)
		}
	}
	if find(items, "Loads", "load → bronze.payments") == nil {
		t.Error("an optional reader being absent took the others with it")
	}
}

// A GET, AND ONLY A GET.
func TestThePaletteIndexIsReadOnly(t *testing.T) {
	mux := http.NewServeMux()
	paletteUI().Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/palette", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a POST to the index answered %d", rec.Code)
	}
}

// THE PALETTE DOES THINGS AS WELL AS GOING PLACES. An action is an item with
// an `action` the island runs instead of following the href; the theme is
// the first one, and the others arrive with the screens that own them.
func TestThePaletteOffersToToggleTheTheme(t *testing.T) {
	_, _, items := palette(t, dataUI(catalogFake{}))
	for _, it := range items {
		if it.Group == "Actions" && said(it.Label) == "{palette.theme}" && it.Action == "toggle-theme" {
			return
		}
	}
	t.Errorf("no Actions › theme in %+v", items)
}
