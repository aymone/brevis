package pages

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"brevis.sh/brevis/internal/domain/diagnosis"
	"brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/web/components"
	"brevis.sh/brevis/web/i18n"
)

// Phase is one step of a run's life on the run page's strip: how it was made,
// when it started, how long it ran, how it ended. State is "done", "current",
// "pending" or "failed". Label is a sentence the app says; Detail is data the
// app writes (nil when there is nothing to say).
type Phase struct {
	Key    string       `json:"key"`
	Label  i18n.Msg     `json:"label"`
	Detail *PhaseDetail `json:"detail"`
	State  string       `json:"state"`
}

// PhaseDetail is what a phase has to say, as data. Kind is "at" (when it was
// made), "slot" (the slot it represents), "late" (how long after its slot or
// trigger it started), "waiting" (how long it has waited), "ran" (how long it
// ran) or "ended" (how it ended: State is the run's status).
type PhaseDetail struct {
	Kind  string     `json:"kind"`
	At    *time.Time `json:"at,omitempty"`
	Ms    *int64     `json:"ms,omitempty"`
	State string     `json:"state,omitempty"`
}

func detailAt(kind string, t time.Time) *PhaseDetail { return &PhaseDetail{Kind: kind, At: &t} }

func detailFor(kind string, d time.Duration) *PhaseDetail {
	ms := d.Milliseconds()
	return &PhaseDetail{Kind: kind, Ms: &ms}
}

// PhasesOf reads a run's phases from its own times. The STATUS says which
// phase is now -- a queued run is waiting to start, a running or retrying one
// is running, a finished one is over -- and the times fill in the details:
// the slot, how late it started, the time running (as of now, so the live
// region's refresh moves it), and how it ended.
func PhasesOf(r run.Run, now time.Time) []Phase {
	made := Phase{Key: "scheduled", Label: i18n.M("run.phase.triggered"), State: "done", Detail: detailAt("at", r.CreatedAt)}
	if r.LogicalDate != nil {
		made.Label, made.Detail = i18n.M("run.phase.scheduled"), detailAt("slot", *r.LogicalDate)
	}
	started := Phase{Key: "started", Label: i18n.M("run.phase.started"), State: "pending"}
	running := Phase{Key: "running", Label: i18n.M("run.phase.running"), State: "pending"}
	finished := Phase{Key: "finished", Label: i18n.M("run.phase.finished"), State: "pending"}

	// How late it started: after its SLOT for a scheduled run -- the same
	// number as its `delay` auto param, so the page does not show two delays
	// -- and after the trigger for one made by hand.
	from := r.CreatedAt
	if r.LogicalDate != nil {
		from = *r.LogicalDate
	}
	if r.StartedAt != nil {
		started.Detail = detailFor("late", r.StartedAt.Sub(from))
	}

	switch r.Status {
	case run.StatusCreated, run.StatusQueued, run.StatusPending:
		started.State = "current"
		started.Detail = detailFor("waiting", now.Sub(from))
	case run.StatusRunning, run.StatusRetrying:
		started.State, running.State = "done", "current"
		if r.StartedAt != nil {
			running.Detail = detailFor("ran", now.Sub(*r.StartedAt))
		}
	case run.StatusSuccess, run.StatusFailed, run.StatusCanceled:
		started.State, running.State = "done", "done"
		if r.StartedAt != nil && r.FinishedAt != nil {
			running.Detail = detailFor("ran", r.FinishedAt.Sub(*r.StartedAt))
		}
		finished.State = "done"
		if r.Status != run.StatusSuccess {
			finished.State = "failed"
		}
		finished.Detail = &PhaseDetail{Kind: "ended", State: string(r.Status), At: r.FinishedAt}
	}
	return []Phase{made, started, running, finished}
}

// AutoItem is one auto param as the grid shows it: its key, what it means
// (a sentence the app says) and its value as data -- an instant, a lateness in
// seconds, or a plain Value.
type AutoItem struct {
	Key          string     `json:"key"`
	Note         i18n.Msg   `json:"note"`
	Value        string     `json:"value"`
	At           *time.Time `json:"at"`
	DelaySeconds *int       `json:"delaySeconds"`
}

// autoItems are a run's auto params as the grid shows them, in the order the
// cards had; an absent one is left out, as before.
func autoItems(r run.Run) []AutoItem {
	a := r.Auto
	adjusted := a.AdjustedAt
	items := []AutoItem{
		{Key: "adjusted_at", Note: i18n.M("auto.adjusted_at"), At: &adjusted},
		{Key: "date", Note: i18n.M("auto.date"), Value: a.Date},
		{Key: "delay", Note: i18n.M("auto.delay"), DelaySeconds: &a.DelaySeconds},
	}
	add := func(key string, t *time.Time, note i18n.Msg) {
		if t != nil {
			items = append(items, AutoItem{Key: key, Note: note, At: t})
		}
	}
	add("scheduled_at", a.ScheduledAt, i18n.M("auto.scheduled_at"))
	add("started_at", a.StartedAt, i18n.M("auto.started_at"))
	add("interval_start", a.IntervalStart, i18n.M("auto.interval_start"))
	add("interval_end", a.IntervalEnd, i18n.M("auto.interval_end"))
	items = append(items, AutoItem{Key: "previous_error", Note: i18n.M("auto.previous_error"), Value: fmt.Sprint(a.PreviousError)})
	add("previous_success_at", a.PreviousSuccessAt, i18n.M("auto.previous_success_at"))
	return items
}

// usedParams are the values this run was given, sorted by name.
func usedParams(r run.Run) []components.KVItem {
	names := make([]string, 0, len(r.Params))
	for k := range r.Params {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]components.KVItem, 0, len(names))
	for _, k := range names {
		out = append(out, components.KVItem{Key: k, Value: r.Params[k]})
	}
	return out
}

// showsWhy is whether the head says why the run failed: a failed run, or a
// retrying one with the failure that sent it back.
func showsWhy(p RunPage) bool {
	return p.Cause != nil && (p.Run.Status == run.StatusFailed || p.Run.Status == run.StatusRetrying)
}

// whyTitle names what stopped the run: its step, and how the step exited
// when it did; "The run failed" when it died before any step.
func whyTitle(c *diagnosis.Cause) i18n.Msg {
	if c.Step == "" {
		return i18n.M("run.why.run")
	}
	step := c.Step
	if c.MapIndex != run.Unmapped {
		step += "[" + strconv.Itoa(c.MapIndex) + "]"
	}
	if c.ExitCode != nil {
		return i18n.M("run.why.step_exit", "step", step, "code", *c.ExitCode)
	}
	return i18n.M("run.why.step", "step", step)
}

// whyExcerpt is the lines before the cause and the cause, or the cause alone.
func whyExcerpt(c *diagnosis.Cause) string {
	if len(c.Excerpt) > 0 {
		return strings.Join(c.Excerpt, "\n")
	}
	return c.Line
}

// contextState says why a step has no published context.
func contextState(state string) i18n.Msg {
	switch state {
	case "running":
		return i18n.M("run.context.running")
	case "failed":
		return i18n.M("run.context.failed")
	}
	return i18n.M("run.context.none")
}
