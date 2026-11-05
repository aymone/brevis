package api

import (
	"context"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// CachedFailing keeps r's answer for ttl. The badge is drawn on every page,
// and the list costs ~6 ms with a thousand workflows and fifty thousand runs
// (measured for run-diagnosis); a count that is fifteen seconds old says the
// same thing to whoever reads it.
//
// An error is not kept: the next page asks again.
func CachedFailing(r FailingReader, ttl time.Duration, now func() time.Time) FailingReader {
	return &cachedFailing{
		r:     r,
		list:  cached(r.FailingWorkflows, ttl, now),
		fresh: cached(r.FailingWorkflows, min(ttl, freshTTL), now),
	}
}

// freshTTL is how old the Overview's list may be. The query costs ~100 ms at
// a hundred thousand runs, and every open Overview reads it again whenever the
// event stream says the runs changed (and on a timer only while the stream is
// down), so ten tabs are ten reads per change, a burst of changes more. Three
// seconds lets the tabs that wake together share one read, and is still shorter
// than any change a person is waiting to see leave the list.
const freshTTL = 3 * time.Second

type cachedFailing struct {
	r     FailingReader
	list  func(context.Context) ([]postgres.FailingWorkflow, error)
	fresh func(context.Context) ([]postgres.FailingWorkflow, error)
}

func (c *cachedFailing) FailingWorkflows(ctx context.Context) ([]postgres.FailingWorkflow, error) {
	return c.list(ctx)
}

// FailureStreak is not cached: it is one workflow's, read for the run open on
// the screen, and a stale count beside a fresh failure would contradict it.
func (c *cachedFailing) FailureStreak(ctx context.Context, slug string) (int, error) {
	return c.r.FailureStreak(ctx, slug)
}

// Fresh reads past the badge's cache, for the Overview's list: a workflow that
// just passed must leave it at the next look, not fifteen seconds later. It
// keeps its own, much shorter, copy (freshTTL), so a crowd of open tabs shares
// one read instead of making one each.
func (c *cachedFailing) Fresh(ctx context.Context) ([]postgres.FailingWorkflow, error) {
	return c.fresh(ctx)
}
