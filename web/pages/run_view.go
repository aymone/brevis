package pages

import (
	"time"

	"brevis.sh/brevis/internal/alerts"
	"brevis.sh/brevis/web/i18n"
)

// RunView is the open run as the React app reads it: the same RunPage the
// templates draw, finished on the server (the words in the reader's language,
// the derived states, the numbered log), so the app holds no second reading of
// a run. Lists are never nil.
type RunView struct {
	ID       string   `json:"id"`
	Workflow string   `json:"workflow"`
	Logos    []string `json:"logos"`

	Status string `json:"status"`
	// Attempt counts from 0, as the engine does.
	Attempt int `json:"attempt"`
	// InFlight allows Cancel; Terminal stops the app's polling (a failed run
	// is a retry away from running, so it is not terminal).
	InFlight bool `json:"inFlight"`
	Terminal bool `json:"terminal"`
	// Canceling is a cancel asked and not yet carried out.
	Canceling bool `json:"canceling"`

	// Trigger is how it was made, as the engine writes it ("schedule", "manual"...); the app says it.
	Trigger    string     `json:"trigger"`
	Slot       *time.Time `json:"slot"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	// DurationMs is nil while the run is in flight or never started.
	DurationMs *int64 `json:"durationMs"`

	// Rerun is "" (the workflow is gone), "now" (a click reruns it) or
	// "params" (it declares params, so a drawer is needed).
	Rerun string `json:"rerun"`

	Err    string       `json:"err"`
	Phases []Phase      `json:"phases"`
	Why    *WhyView     `json:"why"`
	Wait   *WaitView    `json:"wait"`
	Step   *StepLineV   `json:"step"`
	Auto   []AutoItem   `json:"auto"`
	Params []KVView     `json:"params"`
	Steps  []StepGroupV `json:"steps"`
	Alerts []AlertView  `json:"alerts"`
	// Context is what each step published, step by step.
	Context []ContextView `json:"context"`
	Log     LogView       `json:"log"`
}

type WhyView struct {
	Title   i18n.Msg `json:"title"`
	Streak  int      `json:"streak"`
	Excerpt string   `json:"excerpt"`
	// HasLine says the cause is a line of the log (Log.CauseN numbers it); one
	// read from the engine's error has no line to go to.
	HasLine bool `json:"hasLine"`
}

type WaitView struct {
	Group   string       `json:"group"`
	Limit   int          `json:"limit"`
	Ahead   int          `json:"ahead"`
	Holders []HolderView `json:"holders"`
	// More is how many holders the three shown leave out.
	More int `json:"more"`
}

type HolderView struct {
	RunID    string    `json:"runId"`
	Workflow string    `json:"workflow"`
	Since    time.Time `json:"since"`
}

type KVView struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

type StepLineV struct {
	Name            string      `json:"name"`
	Status          string      `json:"status"`
	Action          string      `json:"action"`
	Runtime         string      `json:"runtime"`
	RuntimeInferred bool        `json:"runtimeInferred"`
	SDK             string      `json:"sdk"`
	DurationMs      int64       `json:"durationMs"`
	Published       int         `json:"published"`
	Stages          []StageView `json:"stages"`
}

type StageView struct {
	Label   i18n.Cat     `json:"label"`
	State   string       `json:"state"`
	Detail  string       `json:"detail"`
	Ms      *int64       `json:"ms"`
	Numbers []NumberView `json:"numbers"`
}

// NumberView is one number a stage produced: a count the app writes with the reader's separators, or the
// text it came as.
type NumberView struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type StepGroupV struct {
	Node    string        `json:"node"`
	Mapped  bool          `json:"mapped"`
	Status  string        `json:"status"`
	Ok      int           `json:"ok"`
	Total   int           `json:"total"`
	Command i18n.Cat      `json:"command"`
	Rows    []StepRowView `json:"rows"`
}

type StepRowView struct {
	Node       string   `json:"node"`
	Item       string   `json:"item"`
	Status     string   `json:"status"`
	Attempt    int      `json:"attempt"`
	ExitCode   *int     `json:"exitCode"`
	DurationMs int64    `json:"durationMs"`
	Command    i18n.Cat `json:"command"`
	Err        string   `json:"err"`
}

type AlertView struct {
	Tone      string     `json:"tone"`
	Wording   i18n.Msg   `json:"wording"`
	Channel   string     `json:"channel"`
	Attempts  int        `json:"attempts"`
	Err       string     `json:"err"`
	CreatedAt time.Time  `json:"createdAt"`
	Delivered *time.Time `json:"deliveredAt"`
}

type ContextView struct {
	Step  string `json:"step"`
	JSON  string `json:"json"`
	Bytes int    `json:"bytes"`
	// Note says why there is no JSON; null when there is.
	Note *i18n.Msg `json:"note"`
}

type LogView struct {
	Lines  int            `json:"lines"`
	CauseN int            `json:"causeN"`
	Blocks []LogBlockView `json:"blocks"`
}

type LogBlockView struct {
	Step     string        `json:"step"`
	Attempt  int           `json:"attempt"`
	Status   string        `json:"status"`
	ExitCode *int          `json:"exitCode"`
	Running  bool          `json:"running"`
	Items    []LogItemView `json:"items"`
}

type LogItemView struct {
	N     int    `json:"n,omitempty"`
	Text  string `json:"text,omitempty"`
	Level string `json:"level,omitempty"`
	Cause bool   `json:"cause,omitempty"`
	Gap   int    `json:"gap,omitempty"`
}

// NewRunView finishes a RunPage for the app.
func NewRunView(p RunPage, now time.Time) RunView {
	r := p.Run
	v := RunView{
		ID: r.ID.String(), Workflow: r.WorkflowSlug, Logos: nonNil(p.Logos),
		Status:  string(r.Status),
		Attempt: r.Attempt, InFlight: r.Status.InFlight(), Terminal: r.Status.Terminal(),
		Canceling: r.Status.InFlight() && r.CancelRequestedAt != nil,
		Trigger:   r.TriggerType, Slot: r.LogicalDate,
		CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		Err:    r.Err,
		Phases: []Phase{}, Steps: []StepGroupV{}, Alerts: []AlertView{},
		Context: []ContextView{}, Auto: []AutoItem{}, Params: []KVView{},
	}
	if !r.Status.InFlight() && r.StartedAt != nil && r.FinishedAt != nil {
		ms := r.FinishedAt.Sub(*r.StartedAt).Milliseconds()
		v.DurationMs = &ms
	}
	switch {
	case p.Workflow == nil:
	case len(p.Workflow.Params) == 0:
		v.Rerun = "now"
	default:
		v.Rerun = "params"
	}
	v.Phases = PhasesOf(r, now)
	if showsWhy(p) {
		v.Why = &WhyView{Title: whyTitle(p.Cause), Streak: p.Streak, Excerpt: whyExcerpt(p.Cause), HasLine: p.Cause.LineNo > 0}
	}
	if w := p.Wait; w != nil {
		shown := firstHolders(w.Holders)
		wv := &WaitView{Group: w.Group, Limit: w.Limit, Ahead: w.Ahead, Holders: []HolderView{}, More: len(w.Holders) - len(shown)}
		for _, h := range shown {
			wv.Holders = append(wv.Holders, HolderView{RunID: h.RunID.String(), Workflow: h.Workflow, Since: h.Since})
		}
		v.Wait = wv
	}
	if s := p.Step; s != nil {
		sv := &StepLineV{Name: s.Name, Status: s.Status, Action: s.Action, Runtime: runtimeLabel(s.Runtime), RuntimeInferred: s.RuntimeInferred,
			SDK: s.SDK, DurationMs: s.DurationMs, Published: s.Published, Stages: []StageView{}}
		for _, st := range s.Stages {
			stv := StageView{Label: st.Label, State: stageTone(st.State), Detail: st.Detail, Ms: st.Ms, Numbers: []NumberView{}}
			for _, n := range st.Numbers {
				stv.Numbers = append(stv.Numbers, NumberView(n))
			}
			sv.Stages = append(sv.Stages, stv)
		}
		v.Step = sv
	}
	v.Auto = autoItems(r)
	for _, it := range usedParams(r) {
		v.Params = append(v.Params, KVView{Key: it.Key, Value: it.Value})
	}
	for _, g := range StepOutputs(r, p.Logs) {
		gv := StepGroupV{Node: g.Node, Mapped: g.Mapped, Status: g.Status, Ok: g.Ok, Total: g.Total, Command: g.Command, Rows: []StepRowView{}}
		for _, row := range g.Rows {
			gv.Rows = append(gv.Rows, StepRowView{Node: row.NodeID, Item: row.Item, Status: row.Status, Attempt: row.Attempt,
				ExitCode: row.ExitCode, DurationMs: row.DurationMs, Command: row.Command, Err: row.Err})
		}
		v.Steps = append(v.Steps, gv)
	}
	for _, a := range p.Raised {
		v.Alerts = append(v.Alerts, alertView(a))
	}
	for _, e := range ContextOf(r, p.Logs) {
		cv := ContextView{Step: e.Step, JSON: e.JSON, Bytes: e.Bytes}
		if e.JSON == "" {
			note := contextState(e.State)
			cv.Note = &note
		}
		v.Context = append(v.Context, cv)
	}
	v.Log = logView(RunLogOf(p.Logs, p.Cause))
	return v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func alertView(a alerts.Record) AlertView {
	return AlertView{Tone: alertTone(a.State()), Wording: alertWording(a), Channel: a.Channel,
		Attempts: a.Attempts, Err: a.Err, CreatedAt: a.CreatedAt, Delivered: a.DeliveredAt}
}

func logView(l RunLog) LogView {
	out := LogView{Lines: l.Lines, CauseN: l.CauseN, Blocks: []LogBlockView{}}
	for _, b := range l.Blocks {
		bv := LogBlockView{Step: b.Step, Attempt: b.Attempt, Status: b.Status, ExitCode: b.ExitCode, Running: b.Running, Items: []LogItemView{}}
		for _, it := range b.Items {
			bv.Items = append(bv.Items, LogItemView(it))
		}
		out.Blocks = append(out.Blocks, bv)
	}
	return out
}
