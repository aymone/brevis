package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/branding"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func listUI() *api.UI {
	one, every := 0, 0
	at := time.Date(2026, 10, 15, 4, 0, 0, 0, time.UTC)
	id := "11111111-0000-0000-0000-000000000001"
	reads := readsFake{
		workflows: []postgres.WorkflowSummary{
			{Slug: "daily_sales", Name: "daily_sales", Project: "analytics", Cron: "0 4 * * *", LastStatus: "success", HasSchedule: true,
				Active: true, Tags: []string{"finance", "sales"}, LastRunID: &id, LastRunAt: &at, TotalRuns: 3},
			{Slug: "daily_refunds", Name: "daily_refunds", Project: "analytics", LastStatus: "failed", HasSchedule: true, Cron: "0 5 * * *", Tags: []string{"finance"}},
			{Slug: "backfill", Name: "backfill", Project: "ops", LastStatus: "running"},
		},
		recent: map[string][]postgres.RecentRun{"daily_sales": {{ID: id, Status: "success", At: at}}},
	}
	defs := mapDefs{one: &one, every: &every, defs: map[string]wf.Workflow{
		"daily_sales": {Slug: "daily_sales", Nodes: []wf.Node{{ID: "build", Run: "dbt build"}}},
	}}
	return api.NewUI(reads, defs, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}

func TestTheWorkflowsListIsFilteredCountedAndPagedOnTheServer(t *testing.T) {
	rec, body := getJSON(t, listUI(), "/api/workflows?tag=finance&sort=workflow&dir=desc", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	rows, _ := body["rows"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["slug"] != "daily_sales" {
		t.Fatalf("rows = %v", rows)
	}
	if body["total"] != float64(3) || body["filtered"] != float64(2) {
		t.Errorf("total %v filtered %v", body["total"], body["filtered"])
	}
	counts := body["counts"].(map[string]any)
	if counts[""] != float64(2) || counts["failing"] != float64(1) {
		t.Errorf("counts = %v", counts)
	}
	f := body["filter"].(map[string]any)
	if f["dir"] != "desc" || f["sort"] != "workflow" || f["page"] != float64(1) {
		t.Errorf("filter = %v", f)
	}
	first := rows[0].(map[string]any)
	if first["steps"] != float64(1) || len(first["recent"].([]any)) != 1 || keyOf(first["schedule"]) != "cron.daily" {
		t.Errorf("row = %v", first)
	}
	if langs := first["runsIn"].([]any); len(langs) != 1 || langs[0].(map[string]any)["key"] != "dbt" {
		t.Errorf("runsIn = %v", first["runsIn"])
	}
}

// A page past the end, and no workflow at all, are empty arrays and not null.
func TestTheWorkflowsListNeverSendsNull(t *testing.T) {
	_, body := getJSON(t, listUI(), "/api/workflows?q=nothing-matches&page=9", nil)
	for _, k := range []string{"rows", "tags", "langs"} {
		if _, ok := body[k].([]any); !ok {
			t.Errorf("%s = %v; want an array", k, body[k])
		}
	}
	if row := func() any {
		_, b := getJSON(t, listUI(), "/api/workflows", nil)
		return b["rows"].([]any)[0]
	}(); row.(map[string]any)["tags"] == nil || row.(map[string]any)["recent"] == nil {
		t.Error("a row's lists are null")
	}
}

func TestTheWorkflowsListFailsAsJSON(t *testing.T) {
	ui := api.NewUI(failingReads{}, nil, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
	rec, body := getJSON(t, ui, "/api/workflows", nil)
	if rec.Code != http.StatusInternalServerError || body["error"] == "" {
		t.Errorf("%d %v", rec.Code, body)
	}
}

type failingReads struct{ readsFake }

func (failingReads) Workflows(context.Context) ([]postgres.WorkflowSummary, error) {
	return nil, errors.New("down")
}

type toggleFake struct {
	api.Actions
	active bool
	err    error
	actor  *string
}

func (f toggleFake) Toggle(_ context.Context, _ string, actor string) (bool, error) {
	if f.actor != nil {
		*f.actor = actor
	}
	return f.active, f.err
}

func post(t *testing.T, ui *api.UI, target string, req func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodPost, target, nil)
	if req != nil {
		req(r)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

// The server answers WHAT happened ({active}); the toast's sentence is the screen's, in the reader's language.
func TestTogglingFromTheAppAnswersTheOutcomeNotTheSentence(t *testing.T) {
	actor := ""
	ui := api.NewUI(nil, nil, nil, toggleFake{active: false, actor: &actor}, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	rec := post(t, ui, "/api/workflows/daily/toggle", func(r *http.Request) {
		*r = *r.WithContext(auth.IntoContext(r.Context(), "ana"))
	})
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %v %s", rec.Code, err, rec.Body)
	}
	if got["active"] != false || len(got) != 1 || actor != "ana" {
		t.Errorf("got %v, actor %q; want {active:false} and nothing else", got, actor)
	}
}

func TestTogglingAnUnknownScheduleIs404AsJSON(t *testing.T) {
	ui := api.NewUI(nil, nil, nil, toggleFake{err: pgx.ErrNoRows}, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	rec := post(t, ui, "/api/workflows/ghost/toggle", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	ui = api.NewUI(nil, nil, nil, toggleFake{err: errors.New("db")}, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	if rec := post(t, ui, "/api/workflows/x/toggle", nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("%d", rec.Code)
	}
}

// detailReads adds the reads only the workflow page makes.
type detailReads struct {
	readsFake
	history []postgres.RunSummary
	days    []postgres.Day
	day     **time.Time
	load    []postgres.LoadDay
}

func (d detailReads) LoadTrend(context.Context, string, int) ([]postgres.LoadDay, error) {
	return d.load, nil
}

func (d detailReads) WorkflowHistory(_ context.Context, _ string, day *time.Time, _ int) ([]postgres.RunSummary, error) {
	if d.day != nil {
		*d.day = day
	}
	return d.history, nil
}
func (d detailReads) IndicatorsFor(context.Context, time.Duration, string) (postgres.Indicators, error) {
	return postgres.Indicators{Running: 1, Pending: 2, MeanDuration: 1500 * time.Millisecond}, nil
}
func (d detailReads) RunsPerDay(context.Context, string, int) ([]postgres.Day, error) {
	return d.days, nil
}

func detailUI(def wf.Workflow, day **time.Time) *api.UI {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dur := 2 * time.Second
	logical := today
	reads := detailReads{
		readsFake: readsFake{workflows: []postgres.WorkflowSummary{
			{Slug: def.Slug, Project: "analytics", Cron: def.Schedule, Timezone: "America/Sao_Paulo", HasSchedule: true, Active: true},
		}},
		history: []postgres.RunSummary{
			{ID: "r1", Status: "failed", TriggerType: "manual", LogicalDate: &logical, Duration: &dur, CreatedAt: today,
				Params: map[string]string{"layers": "all", "mode": "x"}},
		},
		days: []postgres.Day{{Date: today, Total: 2, Failed: 1}, {Date: today.AddDate(0, 0, -1), Total: 1, Succeeded: 1}},
		day:  day,
	}
	return api.NewUI(reads, defsFake{w: def}, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}

func TestTheWorkflowPageIsOneJSONDocument(t *testing.T) {
	def := diamond()
	def.Schedule = "0 4 * * *"
	def.Params = []wf.Param{{Name: "mode", Default: "fast"}}
	var day *time.Time
	rec, body := getJSON(t, detailUI(def, &day), "/api/workflows/diamond?day=2026-10-15", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if day == nil || day.Format("2006-01-02") != "2026-10-15" || body["day"] != "2026-10-15" {
		t.Errorf("the day did not reach the history: %v", day)
	}
	if body["project"] != "analytics" || body["active"] != true || body["edges"] != float64(4) || body["nextRun"] == nil {
		t.Errorf("summary = %v", body)
	}
	if steps := body["steps"].([]any); len(steps) != 4 || steps[0].(map[string]any)["id"] != "a" {
		t.Errorf("steps = %v", body["steps"])
	}
	st := body["stats"].(map[string]any)
	if st["ranDays"] != float64(2) || st["failedDays"] != float64(1) || st["running"] != float64(1) || st["queued"] != float64(2) || st["meanMs"] != float64(1500) {
		t.Errorf("stats = %v", st)
	}
	cal := body["calendar"].(map[string]any)
	if len(cal["days"].([]any)) != 2 || cal["failedDays"] != float64(1) {
		t.Errorf("calendar = %v", cal)
	}
	run := body["runs"].([]any)[0].(map[string]any)
	if run["slot"] != true || run["trigger"] != "manual" || run["durationMs"] != float64(2000) {
		t.Errorf("run = %v", run)
	}
	// Only what differs from the defaults: layers is not declared, mode differs.
	if g := run["given"].([]any); len(g) != 2 || g[0] != "layers=all" {
		t.Errorf("given = %v", run["given"])
	}
	if body["yaml"] == "" || len(body["params"].([]any)) != 1 {
		t.Error("yaml or params missing")
	}
}

// The "What it loads" panel: the series the server page draws, null without history.
func TestTheWorkflowJSONCarriesTheLoadTrendWhenThereIsHistory(t *testing.T) {
	def := wf.Workflow{Slug: "solo", Nodes: []wf.Node{{ID: "only", Run: "python x.py"}}}
	_, none := getJSON(t, detailUI(def, nil), "/api/workflows/solo", nil)
	if v, ok := none["load"]; !ok || v != nil {
		t.Errorf("load without history = %v, %v", v, ok)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	withLoad := api.NewUI(detailReads{
		readsFake: readsFake{workflows: []postgres.WorkflowSummary{{Slug: "solo"}}},
		load: []postgres.LoadDay{
			{Date: today.AddDate(0, 0, -1), Runs: 1, Rows: 1000, BytesOut: 2000, ExtractMs: 1000, LoadMs: 2000},
			{Date: today, Runs: 1, Rows: 2000, BytesOut: 4000, ExtractMs: 1000, LoadMs: 2000},
		},
	}, defsFake{w: def}, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
	_, body := getJSON(t, withLoad, "/api/workflows/solo", nil)
	load, _ := body["load"].(map[string]any)
	series, _ := load["series"].([]any)
	if len(series) != 4 || load["from"] == "" || load["to"] == "" {
		t.Fatalf("load = %v", body["load"])
	}
	first := series[0].(map[string]any)
	if first["key"] != "rows" || first["unit"] != "count" || first["latest"] != 2000.0 || first["delta"] != "pct" || first["pct"] != 100.0 || len(first["points"].([]any)) != 2 {
		t.Errorf("first series = %v", first)
	}
}

func TestAOneStepWorkflowIsOneStepInTheList(t *testing.T) {
	def := wf.Workflow{Slug: "solo", Nodes: []wf.Node{{ID: "only", Run: "python x.py"}}}
	_, body := getJSON(t, detailUI(def, nil), "/api/workflows/solo", nil)
	steps := body["steps"].([]any)
	if len(steps) != 1 || steps[0].(map[string]any)["command"].(map[string]any)["v"].([]any)[0] != "python x.py" {
		t.Errorf("steps = %v", steps)
	}
}

func TestAnUnknownWorkflowIs404AsJSON(t *testing.T) {
	ui := api.NewUI(readsFake{}, defsFake{err: errors.New("missing")}, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
	rec, body := getJSON(t, ui, "/api/workflows/ghost", nil)
	if rec.Code != http.StatusNotFound || body["error"] == "" {
		t.Errorf("%d %v", rec.Code, body)
	}
}
