package pages

import (
	"context"
	"time"

	"brevis.sh/brevis/internal/alerts"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
	"brevis.sh/brevis/web/i18n"
)

// ---------------------------------------------------------------------------
// Workflows — a lista de DAGs
// ---------------------------------------------------------------------------

// Filter holds the search bar's state. It lives in the URL, not in memory: that
// way an applied filter can be pasted into the on-call chat and opens the same
// on the other
// lado.
type Filter struct {
	Search string
	// State is one of the segment's questions (failing, running, paused,
	// manual), or a run state an older link carries (success, failed...),
	// which still filters, as a chip.
	State string
	// Active is the schedule filter older links carry: active | paused.
	Active string
	// Lang is a language or tool the workflow runs, as a logo key.
	Lang string

	// Tags is a set: a workflow must carry ALL of them. One `tag=` each in the
	// URL, in the order they were chosen.
	Tags []string

	// Project scopes the list to one project's workflows. It is what makes a
	// project more than a row on a screen: the slug is unique WITHIN a project,
	// so two of them can each have a `daily_ingest`, and until this existed
	// there was no way to look at one project's half of that.
	Project string

	// Sort is the sort field; Desc inverts it. They live in the URL for the same
	// reason the filters do: a sorted screen has to be pasteable to somebody
	// else.
	Sort string
	Desc bool

	Page    int
	PerPage int
}

// Workflow shows the published definition. With no run state: it is the screen
// for "what this workflow is", and it has to serve a workflow that never ran
// too.
// WorkflowStats is one workflow's numbers, over workflowWindow.
//
// Every field degrades to its zero value on purpose: a definition has to stay
// readable with the database in trouble, and somebody looking at a broken
// installation is exactly who needs to read the graph.
type WorkflowStats struct {
	Window time.Duration
	Ind    postgres.Indicators
	Days   []postgres.Day

	// Load is what this workflow's SDK pipelines measured, per day. Empty for a
	// workflow whose steps are shell commands, which is why the panel checks
	// before drawing: four empty boxes would be four questions with no answer.
	Load []postgres.LoadDay
}

// alertTone borrows the run states' palette rather than inventing colours for a
// second vocabulary. The words stay the alert's own.
func alertTone(state string) string {
	switch state {
	case "delivered":
		return "success"
	case "undelivered":
		return "failed"
	}
	return "running"
}

// alertWording says what happened in the reader's terms, not the table's.
//
// "sending" on its own is ambiguous in the way that matters: it is also what a
// row looks like when nothing is draining the outbox, so it says which process
// is supposed to.
func alertWording(ctx context.Context, a alerts.Record) string {
	subject := i18n.T(ctx, "run.alert.run")
	if a.Kind == alerts.KindStep && a.NodeID != "" {
		subject = i18n.T(ctx, "run.alert.step", "step", a.NodeID)
	}
	switch a.State() {
	case "delivered":
		return i18n.T(ctx, "run.alert.delivered", "subject", subject)
	case "undelivered":
		return i18n.T(ctx, "run.alert.undelivered", "subject", subject)
	}
	return i18n.T(ctx, "run.alert.waiting", "subject", subject)
}

// firstHolders is the three a line has room for, oldest first.
func firstHolders(hs []postgres.Holder) []postgres.Holder {
	if len(hs) > 3 {
		return hs[:3]
	}
	return hs
}

// lateness reads as a duration rather than as a count of seconds. "2220" is a
// number somebody converts; "37m" is an answer.
func lateness(seconds int) string {
	if seconds <= 0 {
		return "on time"
	}
	d := time.Duration(seconds) * time.Second
	return components.Duration(&d)
}

// DefaultPerPage is the lists' page size. Twenty-five fits a laptop screen
// without endless scrolling and without forcing a page turn on every glance.
const DefaultPerPage = 25

// InFlight is what "Running" lists: a run waiting in the queue, or for its
// retry, is as unfinished as one whose step is going.
var InFlight = []string{"running", "queued", "retrying"}

// ProximaExecucao is a future trigger, already computed from the cron.
type NextRun struct {
	Workflow string
	Cron     string
	Timezone string
	When     time.Time
	// Logos are what the workflow runs in.
	Logos []string
}

// ---------------------------------------------------------------------------
// Runs, projects and detail
// ---------------------------------------------------------------------------

// RunFilter is the run screen's slice. It arrives through the query string, so
// a dashboard chart can point at "that hour's runs" with a link.
type RunFilter struct {
	State string
	// All says the operator picked "All" -- without it, no state means "the
	// default": Running when something is in flight, else All.
	All      bool
	Workflow string
	De       string // YYYY-MM-DDTHH, the same format the chart emits
	Ate      string
	Label    string // descricao legivel do periodo, montada no servidor

	Page    int
	PerPage int
}
