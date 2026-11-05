package api

import (
	"net/http"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/pages"
)

// runView is a run as a list draws it. Times are RFC3339; a missing one is
// null, never a zero date.
type runView struct {
	ID          string     `json:"id"`
	Workflow    string     `json:"workflow"`
	Status      string     `json:"status"`
	Trigger     string     `json:"trigger"`
	Attempt     int        `json:"attempt"`
	LogicalDate *time.Time `json:"logicalDate"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt"`
	DurationMs  *int64     `json:"durationMs"`
	Error       string     `json:"error"`
	// Logos are the keys of what the workflow runs in.
	Logos []string `json:"logos"`
}

func runViews(runs []postgres.RunSummary, logos map[string][]string) []runView {
	out := make([]runView, 0, len(runs))
	for _, r := range runs {
		v := runView{
			ID: r.ID, Workflow: r.WorkflowSlug, Status: r.Status, Trigger: r.TriggerType, Attempt: r.Attempt,
			LogicalDate: r.LogicalDate, CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, Error: r.Err,
			Logos: orEmpty(logos[r.WorkflowSlug]),
		}
		if r.Duration != nil {
			ms := r.Duration.Milliseconds()
			v.DurationMs = &ms
		}
		out = append(out, v)
	}
	return out
}

type runsFilterView struct {
	// State is "" for All. With no state in the request it is the default the
	// page picks: running while anything is in flight, else all.
	State    string `json:"state"`
	Workflow string `json:"workflow"`
	From     string `json:"from"`
	To       string `json:"to"`
	// Period says the from/to window in words, "" with none.
	Period string `json:"period"`
}

type runsCountsView struct {
	All    int `json:"all"`
	Going  int `json:"going"`
	Failed int `json:"failed"`
}

type runsView struct {
	Runs    []runView      `json:"runs"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"perPage"`
	Pages   int            `json:"pages"`
	Filter  runsFilterView `json:"filter"`
	// Counts are per state with the same filter minus its state, so each
	// choice says what it would show.
	Counts runsCountsView `json:"counts"`
	// ByState is the counts for states the segment does not offer (canceled,
	// queued...): the filter chip names them.
	ByState map[string]int `json:"byState"`
}

// runsAPI is the Runs list: the page of runs a filter selects, how many there
// are and the counts per state. The filter, its default and the paging are the
// server's, as /runs reads them.
func (u *UI) runsAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := pages.RunFilter{
		State: validState(q.Get("state")), Workflow: q.Get("workflow"),
		De: q.Get("from"), Ate: q.Get("to"), Page: page(q.Get("page")), PerPage: pages.DefaultPerPage,
	}
	de, ate := instant(f.De), instant(f.Ate)
	if de == nil {
		f.De = ""
	}
	if ate == nil {
		f.Ate = ""
	}
	query := postgres.RunFilter{
		State: f.State, Workflow: f.Workflow, De: de, Ate: ate,
		Limite: f.PerPage, Offset: (f.Page - 1) * f.PerPage,
	}
	if f.State == "running" {
		query.State, query.States = "", pages.InFlight
	}
	runs, total, counts, err := u.runsList(r.Context(), query)
	if err != nil {
		u.log.Error("listing the runs", "error", err)
		apiError(w, http.StatusInternalServerError, "the runs are unavailable")
		return
	}
	// No state in the address: running while anything is in flight, else all.
	if !q.Has("state") && inFlight(counts) > 0 {
		f.State = "running"
		query.State, query.States = "", pages.InFlight
		if runs, total, counts, err = u.runsList(r.Context(), query); err != nil {
			u.log.Error("listing the runs", "error", err)
			apiError(w, http.StatusInternalServerError, "the runs are unavailable")
			return
		}
	}

	out := runsView{
		Runs: runViews(runs, u.runsLogos(r.Context(), runs)), Total: total, Page: f.Page, PerPage: f.PerPage,
		Pages:   max(1, (total+f.PerPage-1)/f.PerPage),
		Filter:  runsFilterView{State: f.State, Workflow: f.Workflow, From: f.De, To: f.Ate, Period: periodLabel(de, ate)},
		Counts:  runsCountsView{Going: inFlight(counts), Failed: counts["failed"]},
		ByState: map[string]int{},
	}
	for s, n := range counts {
		out.Counts.All += n
		out.ByState[s] = n
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
