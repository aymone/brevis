package api_test

import (
	"context"
	"log/slog"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// runsReads answers the Runs list: the page's runs and the counts per state.
// asked keeps the filter the counts were read with.
type runsReads struct {
	readsFake
	runs   []postgres.RunSummary
	counts map[string]int
	asked  *postgres.RunFilter
	// listed keeps the filter the page was read with; total stands in for a
	// longer list than the page holds.
	listed *postgres.RunFilter
	total  int
}

func (f runsReads) Runs(_ context.Context, q postgres.RunFilter) ([]postgres.RunSummary, error) {
	if f.listed != nil {
		*f.listed = q
	}
	return f.runs, nil
}
func (f runsReads) CountRuns(context.Context, postgres.RunFilter) (int, error) {
	if f.total > 0 {
		return f.total, nil
	}
	return len(f.runs), nil
}
func (f runsReads) CountRunsByState(_ context.Context, q postgres.RunFilter) (map[string]int, error) {
	// The sidebar badge counts with no filter at all; only the list's is recorded.
	if f.asked != nil && q.Workflow != "" {
		*f.asked = q
	}
	return f.counts, nil
}

func dur(d time.Duration) *time.Duration { return &d }

func runsUI(reads runsReads) *api.UI {
	return api.NewUI(reads, nil, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}
