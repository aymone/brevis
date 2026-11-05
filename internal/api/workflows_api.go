package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

// registerWorkflowsAPI wires the Workflows screens' JSON. One line in
// Registrar, so the screen's routes are read in one place.
func (u *UI) registerWorkflowsAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workflows", u.workflowsJSON)
	mux.HandleFunc("GET /api/workflows/{slug}", u.workflowJSON)
	mux.HandleFunc("POST /api/workflows/{slug}/toggle", u.toggleJSON)
}

// logoView is a language, tool or connector: the logo key. The app names it
// (a brand is its own name; "file" and "gateway" are words it translates).
type logoView struct {
	Key string `json:"key"`
}

func logosOf(keys []string) []logoView {
	out := make([]logoView, 0, len(keys))
	for _, k := range keys {
		out = append(out, logoView{Key: k})
	}
	return out
}

type recentView struct {
	ID     string    `json:"id"`
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

// workflowRowView is one row of the list. Steps is null when the definition
// could not be read (the old page then draws no step count either).
type workflowRowView struct {
	Slug        string       `json:"slug"`
	Project     string       `json:"project"`
	Steps       *int         `json:"steps"`
	Failing     bool         `json:"failing"`
	InFlight    bool         `json:"inFlight"`
	HasParams   bool         `json:"hasParams"`
	RunsIn      []logoView   `json:"runsIn"`
	Connects    []logoView   `json:"connects"`
	HasSchedule bool         `json:"hasSchedule"`
	Active      bool         `json:"active"`
	Cron        string       `json:"cron"`
	Schedule    *i18n.Msg    `json:"schedule"` // the cron in words, for the app to say
	Timezone    string       `json:"timezone"`
	Catchup     bool         `json:"catchup"`
	NextRun     *time.Time   `json:"nextRun"`
	LastRunAt   *time.Time   `json:"lastRunAt"`
	TotalRuns   int          `json:"totalRuns"`
	Recent      []recentView `json:"recent"`
	Tags        []string     `json:"tags"`
}

type filterView struct {
	Q       string   `json:"q"`
	State   string   `json:"state"`
	Active  string   `json:"active"`
	Lang    string   `json:"lang"`
	Tags    []string `json:"tags"`
	Project string   `json:"project"`
	Sort    string   `json:"sort"`
	Dir     string   `json:"dir"`
	Page    int      `json:"page"`
	PerPage int      `json:"perPage"`
}

type langCountView struct {
	logoView
	Count int `json:"count"`
}

type workflowsView struct {
	Filter   filterView        `json:"filter"`
	Rows     []workflowRowView `json:"rows"`
	Counts   map[string]int    `json:"counts"`
	Tags     []string          `json:"tags"`
	Langs    []langCountView   `json:"langs"`
	Total    int               `json:"total"`
	Filtered int               `json:"filtered"`
}

// workflowsJSON is the list's data: the same filtering, sorting, counting and
// paging the page does (filtrar, sortBy, contar, recortar), already applied.
func (u *UI) workflowsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := u.leitura.Workflows(ctx)
	if err != nil {
		u.log.Error("listing the workflows", "error", err)
		fail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	now := time.Now()
	for i := range all {
		all[i].NextRun = proximaDoWorkflow(all[i], now)
	}

	q := r.URL.Query()
	f := pages.Filter{
		Search:  strings.TrimSpace(q.Get("q")),
		State:   validListState(q.Get("state")),
		Active:  q.Get("active"),
		Lang:    validLang(q.Get("lang")),
		Tags:    tagsOf(q["tag"]),
		Project: q.Get("project"),
		Sort:    validOrder(q.Get("sort")),
		Desc:    q.Get("dir") == "desc",
		Page:    page(q.Get("page")),
		PerPage: pages.DefaultPerPage,
	}
	if f.Active != "active" && f.Active != "paused" {
		f.Active = ""
	}

	facts := u.listFacts(ctx)
	filtered := filtrar(all, f, facts)
	sortBy(filtered, f)
	rows := recortar(filtered, f)

	slugs := make([]string, len(rows))
	for i, wf := range rows {
		slugs[i] = wf.Slug
	}
	// A failed strip is rows without squares, not a failed screen.
	recent, err := u.leitura.RecentStates(ctx, slugs, pages.RecentRuns)
	if err != nil {
		u.log.Warn("reading the recent runs for the Workflows list", "error", err)
	}
	screen := pages.WorkflowsScreen{Rows: rows}
	u.describeRows(ctx, &screen, facts)

	out := workflowsView{
		Filter: filterView{Q: f.Search, State: f.State, Active: f.Active, Lang: f.Lang, Tags: nonNil(f.Tags),
			Project: f.Project, Sort: f.Sort, Dir: dirOf(f.Desc), Page: f.Page, PerPage: f.PerPage},
		Rows:     make([]workflowRowView, 0, len(rows)),
		Counts:   contar(all, f, facts).State,
		Tags:     nonNil(tagsDe(all)),
		Langs:    []langCountView{},
		Total:    len(all),
		Filtered: len(filtered),
	}
	for _, l := range langChoices(all, f, facts) {
		out.Langs = append(out.Langs, langCountView{logoView{Key: l.Key}, l.Count})
	}
	for _, s := range rows {
		row := workflowRowView{
			Slug: s.Slug, Project: s.Project, Failing: screen.Failing[s.Slug], InFlight: inFlightStatus(s.LastStatus),
			HasParams: screen.Params[s.Slug], RunsIn: logosOf(screen.RunsIn[s.Slug]), Connects: logosOf(screen.Connects[s.Slug]),
			HasSchedule: s.HasSchedule, Active: s.Active, Cron: s.Cron, Schedule: pages.CronMsg(s.Cron),
			Timezone: s.Timezone, Catchup: s.Catchup, NextRun: s.NextRun, LastRunAt: s.LastRunAt, TotalRuns: s.TotalRuns,
			Recent: []recentView{}, Tags: nonNil(s.Tags),
		}
		if n, ok := screen.Steps[s.Slug]; ok {
			row.Steps = &n
		}
		for _, rr := range recent[s.Slug] {
			row.Recent = append(row.Recent, recentView{ID: rr.ID, Status: rr.Status, At: rr.At})
		}
		out.Rows = append(out.Rows, row)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func inFlightStatus(s string) bool { return s == "running" || s == "queued" || s == "retrying" }

func dirOf(desc bool) string {
	if desc {
		return "desc"
	}
	return "asc"
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// toggleJSON pauses or resumes a schedule: handler `toggle`'s effect, as JSON.
func (u *UI) toggleJSON(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	active, err := u.actions.Toggle(r.Context(), slug, auth.De(r.Context()))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, codeNoSchedule)
		return
	}
	if err != nil {
		u.log.Error("toggling the schedule", "workflow", slug, "error", err)
		fail(w, http.StatusInternalServerError, codeScheduleFailed)
		return
	}
	u.log.Info("schedule toggled", "workflow", slug, "active", active)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"active": active})
}
