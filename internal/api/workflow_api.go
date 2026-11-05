package api

import (
	"net/http"
	"slices"
	"time"

	appworkflow "brevis.sh/brevis/internal/application/workflow"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

type statsView struct {
	// Days over the last 30 on which the workflow ran, and those with a failure.
	RanDays    int   `json:"ranDays"`
	FailedDays int   `json:"failedDays"`
	MeanMs     int64 `json:"meanMs"`
	Running    int   `json:"running"`
	Queued     int   `json:"queued"`
}

type calendarDayView struct {
	Date   string `json:"date"` // UTC day, YYYY-MM-DD
	Total  int    `json:"total"`
	Failed int    `json:"failed"`
	Open   int    `json:"open"`
}

type calendarView struct {
	Days        []calendarDayView `json:"days"`
	RanDays     int               `json:"ranDays"`
	FailedDays  int               `json:"failedDays"`
	Streak      int               `json:"streak"`
	LastFailure *string           `json:"lastFailure"`
	Since       *string           `json:"since"`
}

type stepView struct {
	ID         string     `json:"id"`
	Command    i18n.Cat   `json:"command"`
	Langs      []logoView `json:"langs"`
	Uses       []logoView `json:"uses"`
	Needs      []string   `json:"needs"`
	LastStatus string     `json:"lastStatus"`
	LastMs     int64      `json:"lastMs"`
}

type workflowRunView struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Trigger     string     `json:"trigger"`
	Slot        bool       `json:"slot"` // a manual run of a past slot
	LogicalDate *time.Time `json:"logicalDate"`
	Given       []string   `json:"given"` // params that differ from the defaults
	DurationMs  *int64     `json:"durationMs"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type paramView struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Warn        string `json:"warn"`
	Group       string `json:"group"`
	Uses        string `json:"uses"`
	// Enum and Pattern are the rules the Run drawer checks a value against
	// before the server does; Enum is never null.
	Enum    []string `json:"enum"`
	Pattern string   `json:"pattern"`
}

// slotView is one past slot of the schedule; the app says its label (when it was, and the window it processes).
type slotView struct {
	At    time.Time `json:"at"`
	Label i18n.Msg  `json:"label"`
}

type workflowDetailView struct {
	Slug        string            `json:"slug"`
	Description string            `json:"description"`
	Project     string            `json:"project"`
	HasSchedule bool              `json:"hasSchedule"`
	Active      bool              `json:"active"`
	Cron        string            `json:"cron"`
	Schedule    *i18n.Msg         `json:"schedule"`
	Timezone    string            `json:"timezone"`
	NextRun     *time.Time        `json:"nextRun"`
	Tags        []string          `json:"tags"`
	Langs       []logoView        `json:"langs"`
	Edges       int               `json:"edges"`
	Stages      int               `json:"stages"`
	Stats       statsView         `json:"stats"`
	Calendar    calendarView      `json:"calendar"`
	Load        *loadView         `json:"load"`
	Steps       []stepView        `json:"steps"`
	Runs        []workflowRunView `json:"runs"`
	Day         *string           `json:"day"`
	Params      []paramView       `json:"params"`
	// Slots are the schedule's last 14 slots, newest first, for the Run
	// drawer's picker; empty for a workflow with no schedule.
	Slots []slotView `json:"slots"`
	YAML  string     `json:"yaml"`
	Now   time.Time  `json:"now"`
}

func dayString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}

// workflowJSON is the workflow page's data. As on the page, only the
// definition is required: the history, statistics and summary each degrade to
// empty rather than failing the screen.
func (u *UI) workflowJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	def, err := u.defs.Definition(ctx, slug)
	if err != nil {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	day := dayOf(r.URL.Query().Get("day"))
	now := time.Now()

	out := workflowDetailView{
		Slug: def.Slug, Description: def.Description, Cron: def.Schedule, Tags: nonNil(def.Tags),
		Schedule: pages.CronMsg(def.Schedule), Langs: logosOf(runsIn(def, 0)),
		Edges: len(def.Edges), Steps: []stepView{}, Runs: []workflowRunView{}, Params: []paramView{},
		Calendar: calendarView{Days: []calendarDayView{}}, Day: dayString(day), Now: now, Slots: []slotView{},
	}

	latest, err := u.reads.WorkflowHistory(ctx, slug, day, 10)
	if err != nil {
		u.log.Warn("the workflow's history is unavailable", "workflow", slug, "error", err)
	}
	for _, rs := range latest {
		rv := workflowRunView{ID: rs.ID, Status: rs.Status, Trigger: rs.TriggerType, Slot: rs.TriggerType == "manual" && rs.LogicalDate != nil,
			LogicalDate: rs.LogicalDate, Given: nonNil(givenParams(def.Params, rs.Params)), CreatedAt: rs.CreatedAt}
		if rs.Duration != nil {
			ms := rs.Duration.Milliseconds()
			rv.DurationMs = &ms
		}
		out.Runs = append(out.Runs, rv)
	}

	if indicators, err := u.reads.IndicatorsFor(ctx, workflowWindow, slug); err == nil {
		out.Stats.MeanMs, out.Stats.Running, out.Stats.Queued = indicators.MeanDuration.Milliseconds(), indicators.Running, indicators.Pending
	} else {
		u.log.Warn("the workflow's indicators are unavailable", "workflow", slug, "error", err)
	}
	// 53 weeks and the days of this one: the calendar's whole grid.
	if days, err := u.reads.RunsPerDay(ctx, slug, 53*7); err == nil {
		out.Calendar, out.Stats.RanDays, out.Stats.FailedDays = calendarOf(days, now)
	} else {
		u.log.Warn("the workflow's calendar is unavailable", "workflow", slug, "error", err)
	}

	if load, err := u.reads.LoadTrend(ctx, slug, components.TrendDays); err == nil {
		out.Load = loadOf(load)
	} else {
		u.log.Warn("the workflow's load trend is unavailable", "workflow", slug, "error", err)
	}

	if all, err := u.reads.Workflows(ctx); err == nil {
		for _, s := range all {
			if s.Slug != slug {
				continue
			}
			out.Project, out.HasSchedule, out.Active, out.Timezone = s.Project, s.HasSchedule, s.Active, s.Timezone
			out.NextRun = nextOfWorkflow(s, now)
		}
	} else {
		u.log.Warn("the workflow's summary is unavailable", "workflow", slug, "error", err)
	}

	if y, err := appworkflow.YAML(def); err == nil {
		out.YAML = string(y)
	}
	for _, p := range def.Params {
		out.Params = append(out.Params, paramView{Name: p.Name, Type: string(p.Type), Default: p.Default, Title: p.Title,
			Description: p.Description, Warn: p.Warn, Group: p.Group, Uses: p.Uses, Enum: nonNil(p.Enum), Pattern: p.Pattern})
	}
	// The zone is the summary's, read above; without a summary the picker
	// reads the cron in UTC, as the old page did.
	for _, sl := range pages.RecentSlots(def.Schedule, out.Timezone, now, 14) {
		out.Slots = append(out.Slots, slotView{At: sl.At, Label: pages.SlotLabel(sl, out.Timezone)})
	}

	if oneStep(def) {
		if line := stepLineOf(def, nil); line != nil {
			out.Steps = append(out.Steps, stepView{ID: line.Name, Command: i18n.Cat{line.Action}, Langs: logosOf(runsIn(def, 0)),
				Uses: []logoView{}, Needs: []string{}})
		}
		out.Stages = 1
	} else {
		var pg pages.WorkflowPage
		u.stepsOf(ctx, def, &pg)
		out.Stages = pg.Stages
		for _, s := range pg.Steps {
			out.Steps = append(out.Steps, stepView{ID: s.ID, Command: s.Command, Langs: logosOf(s.Langs), Uses: logosOf(s.Uses),
				Needs: nonNil(s.Needs), LastStatus: s.LastStatus, LastMs: s.LastMs})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// calendarOf is the year of runs as days plus the summary the page prints
// (components.SummariseCalendar, so the numbers cannot differ), and how many of the
// last kpiDays days ran and failed.
func calendarOf(days []postgres.Day, now time.Time) (calendarView, int, int) {
	sum := components.SummariseCalendar(days, now)
	cv := calendarView{Days: make([]calendarDayView, 0, len(days)), RanDays: sum.RanDays, FailedDays: sum.FailedDays,
		Streak: sum.Streak, LastFailure: dayString(sum.LastFailure), Since: dayString(sum.Since)}
	ran, failed := 0, 0
	since := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -29)
	for _, d := range days {
		cv.Days = append(cv.Days, calendarDayView{Date: d.Date.UTC().Format("2006-01-02"), Total: d.Total, Failed: d.Failed, Open: d.Open})
		if d.Date.Before(since) || d.Total == 0 {
			continue
		}
		ran++
		if d.Failed > 0 {
			failed++
		}
	}
	return cv, ran, failed
}

// givenParams are the values a run was given that differ from the current
// defaults, as key=value, sorted (pages.givenParams, which is not exported).
func givenParams(declared []wf.Param, given map[string]string) []string {
	defaults := map[string]string{}
	for _, p := range declared {
		defaults[p.Name] = p.Default
	}
	var out []string
	for k, v := range given {
		if d, ok := defaults[k]; !ok || d != v {
			out = append(out, k+"="+v)
		}
	}
	slices.Sort(out)
	return out
}
