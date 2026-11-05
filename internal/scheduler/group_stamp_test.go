package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/queue"
	"brevis.sh/brevis/internal/scheduler"
)

// Every way a run is made carries the group, or the claim cannot hold it.
//
// #32 was a MANUAL run -- the daily, clicked during a slot of the */15 it
// shares models with -- so a group that only scheduled runs carried would fix
// every case but the one reported. Schedule and backfill go through
// createAndEnqueue, which a landing uses too; the click has its own path.
func TestEveryRunOfAGroupedWorkflowCarriesTheGroup(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	project := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO projects (id, slug, name) VALUES ($1, 'p', 'Project')`, project); err != nil {
		t.Fatal(err)
	}
	wRepo := postgres.NewWorkflowRepo(pool)
	if err := wRepo.Publish(ctx, wf.Workflow{
		Slug: "daily", Name: "daily", Kind: wf.KindChain, Schedule: "0 2 * * *",
		MaxActive: 1, Group: "jaffle_models",
		Nodes: []wf.Node{{ID: "a", Run: "echo ok"}},
	}, project, "test"); err != nil {
		t.Fatal(err)
	}
	runs := postgres.NewRunRepo(pool)
	s := scheduler.NewScheduler(postgres.NewScheduleRepo(pool), wRepo, runs, queue.New(pool.Pool),
		noLog(), scheduler.SchedulerOptions{})

	setLastSlot(t, pool, inUTC("2026-01-01T02:00:00Z"))
	if _, err := s.Cycle(ctx, inUTC("2026-01-02T03:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Backfill(ctx, "daily", inUTC("2025-12-01T00:00:00Z"), inUTC("2025-12-01T23:59:00Z"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Trigger(ctx, "daily", inUTC("2026-01-02T03:30:00Z"), nil, nil); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT id, trigger_type FROM runs`)
	if err != nil {
		t.Fatal(err)
	}
	type made struct {
		id      uuid.UUID
		trigger string
	}
	var all []made
	for rows.Next() {
		var m made
		if err := rows.Scan(&m.id, &m.trigger); err != nil {
			t.Fatal(err)
		}
		all = append(all, m)
	}
	rows.Close()

	seen := map[string]bool{}
	for _, m := range all {
		r, err := runs.Get(ctx, m.id)
		if err != nil {
			t.Fatal(err)
		}
		seen[m.trigger] = true
		if r.Group != "jaffle_models" || r.MaxActive != 1 {
			t.Errorf("a %s run carries group %q and limit %d; wanted jaffle_models and 1",
				m.trigger, r.Group, r.MaxActive)
		}
	}
	for _, trigger := range []string{"schedule", "backfill", "manual"} {
		if !seen[trigger] {
			t.Errorf("no %s run was made, so its group was never checked (made: %v)", trigger, seen)
		}
	}
}

// A workflow with no group stores NULL, not ”, and reads back empty: the
// claim's COALESCE has to fall back to the slug for it.
func TestARunWithoutAGroupStoresNull(t *testing.T) {
	s, runs, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	id, err := s.Trigger(ctx, "daily", inUTC("2026-01-02T03:30:00Z"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT concurrency_group IS NULL FROM runs WHERE id = $1`, id).Scan(&isNull); err != nil {
		t.Fatal(err)
	}
	if !isNull {
		t.Error("a run with no group stored a value; the claim would partition it apart from its own workflow")
	}
	if r, err := runs.Get(ctx, id); err != nil || r.Group != "" {
		t.Errorf("group %q, err %v; wanted empty", r.Group, err)
	}
}
