package pages

import (
	"context"
	"time"

	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

// RecentRuns is how many of a workflow's last runs its row draws.
const RecentRuns = 20

// zoneSuffix names a schedule's timezone after its words, when it is not
// UTC: "every day at 04:00" means four in São Paulo for a São Paulo cron.
func zoneSuffix(tz string) string {
	if tz == "" || tz == "UTC" || tz == "Etc/UTC" {
		return ""
	}
	return " (" + tz + ")"
}

// WorkflowsScreen is everything the Workflows list draws.
type WorkflowsScreen struct {
	// Rows are the page's workflows, filtered and sorted.
	Rows   []postgres.WorkflowSummary
	Filter Filter
	Counts WorkflowCounts
	// Tags are every workflow's tags, for the popover; Langs what they run.
	Tags  []string
	Langs []LangChoice
	// Recent are each row's last runs.
	Recent map[string][]postgres.RecentRun
	// RunsIn is what each row runs, every language and tool; Connects what
	// it talks to; Steps how many steps; Failing whether its last finished
	// run failed. By slug.
	RunsIn   map[string][]string
	Connects map[string][]string
	Steps    map[string]int
	Failing  map[string]bool
	// Params is whether each row declares parameters, which its Run asks
	// for in the workflow's drawer.
	Params map[string]bool
	// Now is the instant the page is drawn at: the Run dialogs' last slots
	// are up to it.
	Now time.Time
	// Total is every workflow; Filtered those the filter keeps.
	Total, Filtered int
}

// LangChoice is one language of the filter: a logo key, and how many
// workflows the rest of the filter keeps that run it.
type LangChoice struct {
	Key   string
	Count int
}

// WorkflowCounts is how many workflows each state would show, with the rest
// of the filter as it is. A choice's count ignores its own filter --
// "Failing 1" while "Paused" is chosen -- or every other choice would read 0.
// The key "" is the choice that does not filter.
type WorkflowCounts struct {
	State map[string]int
}

// workflowStates are the questions the state segment asks, in its order.
var workflowStates = []struct{ value, key string }{
	{"", "workflows.state.all"},
	{"failing", "workflows.state.failing"},
	{"running", "workflows.state.running"},
	{"paused", "workflows.state.paused"},
	{"manual", "workflows.state.manual"},
}

// IsWorkflowState says whether a state is one of the segment's questions;
// any other is a run state an older link carries.
func IsWorkflowState(s string) bool {
	for _, st := range workflowStates {
		if st.value == s {
			return true
		}
	}
	return false
}

// WorkflowPage is what a workflow's page is drawn from (screen-workflow-v2).
type WorkflowPage struct {
	Def wf.Workflow
	// Sum is the workflow's summary row: project, schedule state, the next
	// run. Nil when it could not be read; the page then says less.
	Sum    *postgres.WorkflowSummary
	Latest []postgres.RunSummary
	Stats  WorkflowStats
	// Graph is the embedded graph JSON; empty for a one-step workflow.
	Graph string
	// Step is a one-step workflow's step as a line, from its definition. Nil
	// when the graph draws it.
	Step *StepLine
	// Now is the page's clock, for "next run in 12m".
	Now time.Time
	// Langs is every language and tool the workflow runs, as logo keys.
	Langs []string
	// Steps are the Steps card's list, in the graph's order; Stages how many
	// columns the graph has; GraphLogos the logos the island borrows.
	Steps      []StepItem
	Stages     int
	GraphLogos []string
	// YAML is the Definition tab: the published workflow written back as a
	// file. Empty when it could not be written.
	YAML string
	// Day narrows the Runs tab to one UTC day -- a square of the calendar;
	// nil is the last runs.
	Day *time.Time
}

// RunNow is what the Run dialog needs: the workflow, its schedule, and the
// page to come back to.
type RunNow struct {
	Slug, Cron, Zone string
	Back             string
	Now              time.Time
}

// slotLabel is a slot as the Run dialog lists it: when, in the schedule's
// zone, and the window it processes, in UTC like every auto param.
func slotLabel(ctx context.Context, s Slot, zone string) string {
	at := s.At
	if loc, err := time.LoadLocation(zone); err == nil && zone != "" {
		at = at.In(loc)
	}
	return i18n.T(ctx, "workflows.slot.option",
		"slot", i18n.Date(ctx, at, "brief"),
		"from", i18n.Date(ctx, s.From, "brief"), "to", i18n.Date(ctx, s.At, "brief"))
}

// StepItem is one step of the Steps card's list.
type StepItem struct {
	ID, Command string
	// Langs is what it runs, language first; Uses what it wrote, as logo keys;
	// Needs the steps it depends on.
	Langs, Uses, Needs []string
	// LastStatus and LastMs are how it went the last time it ran; empty and
	// 0 when it never did.
	LastStatus string
	LastMs     int64
}

// ParamGroup is the params of one group, in the drawer's order.
type ParamGroup struct {
	Name   string
	Params []wf.Param
}
