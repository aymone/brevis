package pages

import (
	"brevis.sh/brevis/internal/domain/diagnosis"

	"brevis.sh/brevis/internal/alerts"
	"brevis.sh/brevis/internal/domain/run"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// RunPage is what the run page and its live refresh are drawn from. One
// struct, filled by one helper in the handler, so the page and the fragment
// that refreshes it cannot read a run differently.
type RunPage struct {
	Run    run.Run
	Logs   []postgres.StepLog
	Raised []alerts.Record
	Wait   *postgres.Wait

	// Graph is the embedded graph JSON (nav-latency NL5). Only the full page
	// carries it; the live fragment does not redraw the graph.
	Graph string

	// Step is a one-step run's step, drawn as a line where a multi-step run
	// has its graph (screen-run-v2 RV3). Nil for every other run.
	Step *StepLine

	// Workflow is the CURRENT definition, for Rerun: what the trigger resolves
	// against. Nil when the workflow is gone, and then there is no Rerun.
	Workflow *wf.Workflow

	// Logos is what the workflow runs in, as the Runs list shows it.
	Logos []string

	// Cause is why a failed or retrying run failed; nil when there is nothing
	// to say. Streak is how many times in a row its workflow failed.
	Cause  *diagnosis.Cause
	Streak int
}
