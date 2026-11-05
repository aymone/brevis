package api_test

import (
	"context"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// workflowReads answers what a workflow's page reads: the summary row (for
// its project and schedule state), its runs and its numbers.
type workflowReads struct {
	readsFake
	ind  postgres.Indicators
	runs []postgres.RunSummary
	days []postgres.Day
	// day is the day the history was asked for, when one was.
	day **time.Time
}

func (f workflowReads) WorkflowHistory(_ context.Context, _ string, day *time.Time, _ int) ([]postgres.RunSummary, error) {
	if f.day != nil {
		*f.day = day
	}
	return f.runs, nil
}
func (f workflowReads) IndicatorsFor(context.Context, time.Duration, string) (postgres.Indicators, error) {
	return f.ind, nil
}
func (f workflowReads) RunsPerDay(context.Context, string, int) ([]postgres.Day, error) {
	return f.days, nil
}
func (f workflowReads) LoadTrend(context.Context, string, int) ([]postgres.LoadDay, error) {
	return nil, nil
}
