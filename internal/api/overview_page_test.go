package api_test

import (
	"context"
	"log/slog"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// overviewReads answers what the Overview reads, and keeps the window and
// grain it was asked for.
type overviewReads struct {
	readsFake
	indicators postgres.Indicators
	inFlight   []postgres.RunSummary
	recent     []postgres.RunSummary
	schedules  []postgres.ScheduleSummary
	asked      *struct {
		span  time.Duration
		grain postgres.Grain
		n     int
	}
}

func (f overviewReads) Indicators(_ context.Context, w time.Duration) (postgres.Indicators, error) {
	f.asked.span = w
	return f.indicators, nil
}
func (f overviewReads) RunsPer(_ context.Context, g postgres.Grain, n int) ([]postgres.Bucket, error) {
	f.asked.grain, f.asked.n = g, n
	out := make([]postgres.Bucket, n)
	for i := range out {
		out[i].Start = time.Now().Add(-time.Duration(n-1-i) * 24 * time.Hour)
	}
	return out, nil
}
func (f overviewReads) InFlight(context.Context, int) ([]postgres.RunSummary, error) {
	return f.inFlight, nil
}
func (f overviewReads) LatestRuns(context.Context, int) ([]postgres.RunSummary, error) {
	return f.recent, nil
}
func (f overviewReads) QueueDepth(context.Context) (int, int, error) { return 2, 0, nil }
func (f overviewReads) Schedules(context.Context) ([]postgres.ScheduleSummary, error) {
	return f.schedules, nil
}

func overviewUI(f overviewReads) *api.UI {
	if f.asked == nil {
		f.asked = &struct {
			span  time.Duration
			grain postgres.Grain
			n     int
		}{}
	}
	return api.NewUI(f, nil, nil, nil, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
}
