package api_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

type failingFake struct {
	n   int
	err error
}

func (f failingFake) FailureStreak(context.Context, string) (int, error) { return 0, nil }

func (f failingFake) FailingWorkflows(context.Context) ([]postgres.FailingWorkflow, error) {
	return make([]postgres.FailingWorkflow, f.n), f.err
}

type runsFake struct {
	api.Leitura
	counts map[string]int
	err    error
}

func (r runsFake) CountRunsByState(context.Context, postgres.RunFilter) (map[string]int, error) {
	return r.counts, r.err
}

func (r runsFake) CountRunsInFlight(context.Context) (int, error) {
	return r.counts["running"] + r.counts["queued"] + r.counts["retrying"], r.err
}

func withRuns(r runsFake) *api.UI {
	return api.NewUI(r, nil, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler)).WithEndpoints(endpointsFake{})
}

type countingFailing struct {
	reads *int
	err   error
}

func (c countingFailing) FailureStreak(context.Context, string) (int, error) { return 0, nil }

func (c countingFailing) FailingWorkflows(context.Context) ([]postgres.FailingWorkflow, error) {
	*c.reads++
	return make([]postgres.FailingWorkflow, *c.reads), c.err
}

// THE COUNT IS READ AT MOST ONCE PER TTL: it costs ~6 ms with a thousand
// workflows, and every page draws it. An answer is kept for the TTL; an
// error is not, so the next page tries again.
func TestTheFailingCountIsKeptForItsTTL(t *testing.T) {
	reads, now := 0, time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)
	cached := api.CachedFailing(countingFailing{reads: &reads}, 15*time.Second, func() time.Time { return now })
	ctx := context.Background()

	a, _ := cached.FailingWorkflows(ctx)
	now = now.Add(14 * time.Second)
	b, _ := cached.FailingWorkflows(ctx)
	if reads != 1 || len(a) != 1 || len(b) != 1 {
		t.Errorf("within the TTL: %d reads, answers %d and %d; want 1 read", reads, len(a), len(b))
	}
	now = now.Add(2 * time.Second)
	if c, _ := cached.FailingWorkflows(ctx); reads != 2 || len(c) != 2 {
		t.Errorf("after the TTL: %d reads, answer %d; want a second read", reads, len(c))
	}

	failing := 0
	broken := api.CachedFailing(countingFailing{reads: &failing, err: errors.New("down")}, 15*time.Second, func() time.Time { return now })
	_, _ = broken.FailingWorkflows(ctx)
	if _, err := broken.FailingWorkflows(ctx); err == nil || failing != 2 {
		t.Errorf("an error was kept: %d reads, err %v", failing, err)
	}
}

// THE IN-FLIGHT COUNT IS READ AT MOST ONCE PER TTL, errors not kept: every
// page and every open tab's poll draws it.
func TestTheInFlightCountIsKeptForItsTTL(t *testing.T) {
	reads, now := 0, time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)
	var fail error
	cached := api.CachedInFlight(func(context.Context) (int, error) { reads++; return reads, fail }, 5*time.Second, func() time.Time { return now })
	ctx := context.Background()

	a, _ := cached(ctx)
	now = now.Add(4 * time.Second)
	b, _ := cached(ctx)
	if reads != 1 || a != 1 || b != 1 {
		t.Errorf("within the TTL: %d reads, answers %d and %d; want 1 read", reads, a, b)
	}
	now = now.Add(2 * time.Second)
	if c, _ := cached(ctx); reads != 2 || c != 2 {
		t.Errorf("after the TTL: %d reads, answer %d; want a second read", reads, c)
	}
	now, fail = now.Add(6*time.Second), errors.New("down")
	if _, err := cached(ctx); err == nil {
		t.Fatal("the error was swallowed")
	}
	fail = nil
	if c, err := cached(ctx); err != nil || c != 4 {
		t.Errorf("after an error: %d, %v; want a fresh read (4)", c, err)
	}
}
