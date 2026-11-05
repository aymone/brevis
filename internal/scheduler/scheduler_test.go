package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	dom "brevis.sh/brevis/internal/domain/run"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/queue"
	"brevis.sh/brevis/internal/scheduler"
)

func inUTC(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// build prepares the project, a published workflow and the scheduler.
func build(t *testing.T, cron string, catchup bool) (*scheduler.Scheduler, *postgres.RunRepo, *queue.Queue, *postgres.Pool) {
	t.Helper()
	pool := testDB(t)
	ctx := context.Background()

	project := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO projects (id, slug, name) VALUES ($1, 'p', 'Projeto')`, project); err != nil {
		t.Fatal(err)
	}

	wRepo := postgres.NewWorkflowRepo(pool)
	w := wf.Workflow{
		Slug: "diario", Name: "diario", Kind: wf.KindChain, Schedule: cron,
		Nodes: []wf.Node{{ID: "a", Run: "echo ok"}},
	}
	if err := wRepo.Publish(ctx, w, project, "test"); err != nil {
		t.Fatal(err)
	}
	if catchup {
		if _, err := pool.Exec(ctx,
			`UPDATE schedules SET catchup = true WHERE workflow_slug = 'diario'`); err != nil {
			t.Fatal(err)
		}
	}

	runs := postgres.NewRunRepo(pool)
	queue := queue.New(pool.Pool)
	s := scheduler.NewScheduler(postgres.NewScheduleRepo(pool), wRepo, runs, queue,
		noLog(), scheduler.SchedulerOptions{})
	return s, runs, queue, pool
}

func setLastSlot(t *testing.T, pool *postgres.Pool, when time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE schedules SET last_slot = $1 WHERE workflow_slug = 'diario'`, when); err != nil {
		t.Fatal(err)
	}
}

// Publishing writes the graph and the schedule together, and the scheduler
// materializes the slot.
func TestTheSchedulerCreatesARunAndEnqueuesIt(t *testing.T) {
	s, runs, queue, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-01-01T02:00:00Z"))

	n, err := s.Cycle(ctx, inUTC("2026-01-02T03:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("created %d runs, wanted 1", n)
	}

	count, _ := runs.CountByStatus(ctx)
	if count[dom.StatusQueued] != 1 {
		t.Errorf("queued = %d, wanted 1", count[dom.StatusQueued])
	}
	byTrigger, _ := runs.CountByTrigger(ctx)
	if byTrigger["schedule"] != 1 {
		t.Errorf("trigger_type = %v, wanted schedule", byTrigger)
	}
	pending, _, _ := queue.Size(ctx)
	if pending != 1 {
		t.Errorf("the queue holds %d, wanted 1 -- the scheduler creates AND enqueues", pending)
	}
}

// The point of the idempotency: re-running the cycle at the same instant must
// not
// duplicar. E o caso da secao 29 — o scheduler cai e sobe de novo.
func TestARepeatedCycleDoesNotDuplicate(t *testing.T) {
	s, runs, _, pool := build(t, "0 2 * * *", true)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-01-01T02:00:00Z"))
	now := inUTC("2026-01-04T03:00:00Z")

	first, err := s.Cycle(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := s.Cycle(ctx, now)
	if err != nil {
		t.Fatal(err)
	}

	if first != 3 { // 02, 03, 04
		t.Errorf("first cycle created %d, wanted 3", first)
	}
	if secondRun != 0 {
		t.Errorf("second cycle created %d, wanted 0", secondRun)
	}
	c, _ := runs.CountByStatus(ctx)
	if total := c[dom.StatusQueued]; total != 3 {
		t.Errorf("total runs = %d, wanted 3", total)
	}
}

// catchup=false materializes only the most recent slot, even with days of gap.
func TestCatchupFalseDoesNotRedoThePast(t *testing.T) {
	s, runs, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-01-01T02:00:00Z"))

	n, err := s.Cycle(ctx, inUTC("2026-01-10T03:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("created %d runs, wanted 1 — catchup=false ignores the gap", n)
	}
	c, _ := runs.CountByStatus(ctx)
	if c[dom.StatusQueued] != 1 {
		t.Errorf("queued = %d, wanted 1", c[dom.StatusQueued])
	}
}

// A backfill enters the queue like any run, with its own trigger and a lower
// priority -- section 12 requires it to respect concurrency and priority.
func TestABackfillEntersTheQueueWithLowerPriority(t *testing.T) {
	s, runs, queue, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-03-01T02:00:00Z"))

	n, err := s.Backfill(ctx, "diario", inUTC("2026-01-01T00:00:00Z"), inUTC("2026-01-05T23:59:00Z"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Created != 5 {
		t.Fatalf("the backfill created %d runs, wanted 5", n)
	}

	byTrigger, _ := runs.CountByTrigger(ctx)
	if byTrigger["backfill"] != 5 {
		t.Errorf("trigger = %v, wanted 5 backfill", byTrigger)
	}
	pending, _, _ := queue.Size(ctx)
	if pending != 5 {
		t.Errorf("the queue holds %d, wanted 5", pending)
	}

	var prio int
	if err := pool.QueryRow(ctx, `SELECT min(priority) FROM queue_items`).Scan(&prio); err != nil {
		t.Fatal(err)
	}
	if prio >= 0 {
		t.Errorf("priority = %d; a backfill has to yield to the current work", prio)
	}
}

// A backfill fills the past and must NOT advance the marker, or the scheduler
// would skip future slots that have not happened yet.
func TestABackfillDoesNotAdvanceTheMarker(t *testing.T) {
	s, _, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	marker := inUTC("2026-03-01T02:00:00Z")
	setLastSlot(t, pool, marker)

	if _, err := s.Backfill(ctx, "diario", inUTC("2026-01-01T00:00:00Z"), inUTC("2026-01-03T23:59:00Z"), nil); err != nil {
		t.Fatal(err)
	}

	var after time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_slot FROM schedules WHERE workflow_slug = 'diario'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(marker) {
		t.Errorf("last_slot = %v, should still be %v", after, marker)
	}
}

// Republishing a workflow must not make the scheduler recreate slots already
// materializados.
func TestRepublishingPreservesTheMarker(t *testing.T) {
	_, _, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	marker := inUTC("2026-05-01T02:00:00Z")
	setLastSlot(t, pool, marker)

	var project uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM projects LIMIT 1`).Scan(&project); err != nil {
		t.Fatal(err)
	}
	w := wf.Workflow{
		Slug: "diario", Name: "diario v2", Kind: wf.KindChain, Schedule: "0 3 * * *",
		Nodes: []wf.Node{{ID: "a", Run: "echo v2"}},
	}
	if err := postgres.NewWorkflowRepo(pool).Publish(ctx, w, project, "test"); err != nil {
		t.Fatal(err)
	}

	var after time.Time
	var cron string
	if err := pool.QueryRow(ctx,
		`SELECT last_slot, cron FROM schedules WHERE workflow_slug = 'diario'`).Scan(&after, &cron); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(marker) {
		t.Errorf("last_slot = %v; republishing must not recreate the past", after)
	}
	if cron != "0 3 * * *" {
		t.Errorf("cron = %q; the new one should win", cron)
	}
}

// Taking `schedule` out of the YAML has to unschedule, and not leave the old
// schedule alive.
func TestPublishingWithNoCronRemovesTheSchedule(t *testing.T) {
	_, _, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()

	var project uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM projects LIMIT 1`).Scan(&project); err != nil {
		t.Fatal(err)
	}
	w := wf.Workflow{Slug: "diario", Name: "diario", Kind: wf.KindChain,
		Nodes: []wf.Node{{ID: "a", Run: "echo"}}}
	if err := postgres.NewWorkflowRepo(pool).Publish(ctx, w, project, "test"); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM schedules WHERE workflow_slug = 'diario'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("the schedule should have been removed")
	}
}

// A one-day backfill with an hourly cron has to give 24 slots, not 23: `Next(t)`
// returns the next one strictly after `t`, so starting exactly at
// `de` excluiria o slot da meia-noite.
func TestABackfillIncludesTheEdgeSlot(t *testing.T) {
	s, _, _, pool := build(t, "0 * * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-06-01T00:00:00Z"))

	n, err := s.Backfill(ctx, "diario",
		inUTC("2026-01-01T00:00:00Z"), inUTC("2026-01-01T23:59:59Z"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Created != 24 {
		t.Errorf("the backfill created %d slots, wanted 24 (00:00 to 23:00)", n.Created)
	}
}

// The folder is the source of truth -- but publishing only ever ADDED. Taking a
// file out of it took nothing out of the database, and the scheduler went on
// materializing runs for a workflow nobody could see any more. With a 15-minute
// cron, that is invisible work running forever.
func TestPruningRemovesWhatLeftTheFolder(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	repo := postgres.NewWorkflowRepo(pool)
	schedules := postgres.NewScheduleRepo(pool)

	var project uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (id, slug, name) VALUES ($1,'acme','acme') RETURNING id`,
		uuid.New()).Scan(&project); err != nil {
		t.Fatal(err)
	}

	stays := wf.Workflow{Slug: "id_verification", Name: "id", Schedule: "0 4 * * *",
		Nodes: []wf.Node{{ID: "run", Run: "dbt build"}}}
	leaving := wf.Workflow{Slug: "vendors_ana_telemetry", Name: "ana", Schedule: "*/15 * * * *",
		Nodes: []wf.Node{{ID: "run", Run: "echo x"}}}
	for _, w := range []wf.Workflow{stays, leaving} {
		if err := repo.Publish(ctx, w, project, "test"); err != nil {
			t.Fatal(err)
		}
	}

	// An old run of what is about to go: the history has to survive.
	if _, err := postgres.NewRunRepo(pool).Create(ctx, dom.Run{
		WorkflowSlug: leaving.Slug, IdempotencyKey: "antiga", Definition: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := repo.Prune(ctx, project, []string{stays.Slug}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != leaving.Slug {
		t.Fatalf("removed = %v, want only %s", removed, leaving.Slug)
	}

	if _, err := repo.Definition(ctx, leaving.Slug); err == nil {
		t.Error("the removed workflow still has a definition in the database")
	}
	if _, err := repo.Definition(ctx, stays.Slug); err != nil {
		t.Errorf("the workflow that stayed is gone: %v", err)
	}

	// The schedule lives in a separate table, linked by slug as text -- the
	// CASCADE does not reach it, and an orphaned schedule would go on creating
	// runs.
	active, err := schedules.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.WorkflowSlug == leaving.Slug {
			t.Error("the removed workflow's schedule survived and would keep creating runs")
		}
	}

	var history int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM runs WHERE workflow_slug = $1`, leaving.Slug).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 1 {
		t.Errorf("the history was deleted along with it (%d runs); deleting the run would be deleting the evidence", history)
	}
}

// With nothing to prune, it touches nothing.
func TestPruningWithNoDifferenceRemovesNothing(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	repo := postgres.NewWorkflowRepo(pool)

	var project uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (id, slug, name) VALUES ($1,'acme','acme') RETURNING id`,
		uuid.New()).Scan(&project); err != nil {
		t.Fatal(err)
	}
	w := wf.Workflow{Slug: "so_esse", Name: "x", Nodes: []wf.Node{{ID: "a", Run: "echo"}}}
	if err := repo.Publish(ctx, w, project, "test"); err != nil {
		t.Fatal(err)
	}

	removed, err := repo.Prune(ctx, project, []string{"so_esse"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Errorf("it removed %v for no reason", removed)
	}
}

// A schedule has no project: it is keyed by the slug alone. Moving a workflow
// to another project -- published there first, then pruned from the old one --
// must not take its cron, its pause or its last slot with it. Pruning deleted
// the schedule by slug, so a paused flow came back active (and with no last
// slot) the next time the new project published it.
func TestPruningKeepsTheScheduleOfASlugAnotherProjectStillPublishes(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	repo := postgres.NewWorkflowRepo(pool)

	project := func(slug string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO projects (id, slug, name) VALUES ($1,$2,$2) RETURNING id`,
			uuid.New(), slug).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	old, moved := project("old"), project("moved")

	w := wf.Workflow{Slug: "billing", Name: "billing", Schedule: "30 4 * * *",
		Nodes: []wf.Node{{ID: "run", Run: "dbt build"}}}
	for _, p := range []uuid.UUID{old, moved} {
		if err := repo.Publish(ctx, w, p, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE schedules SET active = false, last_slot = $2 WHERE workflow_slug = $1`,
		w.Slug, inUTC("2026-01-01T04:30:00Z")); err != nil {
		t.Fatal(err)
	}

	removed, err := repo.Prune(ctx, old, []string{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != w.Slug {
		t.Fatalf("removed = %v, want only %s", removed, w.Slug)
	}

	var active bool
	var last *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT active, last_slot FROM schedules WHERE workflow_slug = $1`, w.Slug,
	).Scan(&active, &last); err != nil {
		t.Fatalf("the schedule went with the pruned copy: %v", err)
	}
	if active || last == nil {
		t.Errorf("schedule = active %v, last slot %v; want the pause and the slot kept", active, last)
	}

	// Pruned from the last project that publishes it, the schedule goes.
	if _, err := repo.Prune(ctx, moved, []string{}, "test"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM schedules WHERE workflow_slug = $1`, w.Slug).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("an orphaned schedule survived the last prune (%d)", n)
	}
}

// A newly published schedule (`last_slot` NULL) has to start firing.
//
// This is the test that was missing, and the gap had a shape: EVERY case above
// calls `setLastSlot` before the cycle, so the null-marker path was never
// exercised. In dev, 18 workflows sat registered for hours with not one
// automatic run -- a `*/30` among them -- because `Slots` started from
// proprio `agora`, o proximo horario do cron era sempre futuro, e o marcador
// never left NULL to break the circle.
func TestANewScheduleStartsFiring(t *testing.T) {
	s, runs, _, _ := build(t, "*/30 * * * *", false)
	ctx := context.Background()

	// First cycle: it only plants the marker, without running the past.
	n, err := s.Cycle(ctx, inUTC("2026-01-01T10:05:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the first cycle created %d runs; the slot before registration is not ours", n)
	}

	// The second cycle, after the clock passed 10:30: now it fires.
	n, err = s.Cycle(ctx, inUTC("2026-01-01T10:31:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("created %d runs after the cron's time; wanted 1", n)
	}
	byTrigger, _ := runs.CountByTrigger(ctx)
	if byTrigger["schedule"] != 1 {
		t.Errorf("trigger = %v; wanted schedule", byTrigger)
	}
}

// The debut marker must not be replanted on every cycle: if it were, `de` would
// advance along with the clock and the schedule would go back to never firing.
func TestTheFirstRunMarkerIsPlantedOnlyOnce(t *testing.T) {
	s, _, _, pool := build(t, "*/30 * * * *", false)
	ctx := context.Background()

	if _, err := s.Cycle(ctx, inUTC("2026-01-01T10:05:00Z")); err != nil {
		t.Fatal(err)
	}
	var first time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_slot FROM schedules WHERE workflow_slug = 'diario'`).Scan(&first); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Cycle(ctx, inUTC("2026-01-01T10:10:00Z")); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_slot FROM schedules WHERE workflow_slug = 'diario'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(first) {
		t.Errorf("the marker moved from %s to %s -- the schedule would never catch up to a time", first, after)
	}
}

// A BACKFILL SAYS WHAT IT SKIPPED, not only what it made.
//
// `brevis backfill` printed `N backfill run(s) queued`, and N is correct: a
// slot that already has a run is not created twice, which is the right
// behaviour and the reason the number can be small. What it did not say is
// how small out of WHAT.
//
// Run the same January twice and the second prints `0 backfill run(s)
// queued`. So does a backfill whose dates are wrong, one whose cron never
// fires in the interval, and one aimed at the wrong workflow. Four different
// situations, one sentence, and only one of them means "this is already
// done" -- so the operator either re-reads the command for a mistake that is
// not there, or stops trusting the number.
//
// The interval's slot count is the context that separates them, and the
// scheduler is the only thing that knows it.
func TestABackfillReportsWhatWasAlreadyThere(t *testing.T) {
	s, _, _, pool := build(t, "0 * * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-06-01T00:00:00Z"))

	from, to := inUTC("2026-01-01T00:00:00Z"), inUTC("2026-01-01T23:59:59Z")
	first, err := s.Backfill(ctx, "diario", from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Slots != 24 || first.Created != 24 || first.Existing() != 0 {
		t.Fatalf("the first backfill reported %+v, want 24 slots all created", first)
	}

	// THE SAME INTERVAL AGAIN. Nothing is created, and that is right --
	// what matters is that the answer can say so.
	again, err := s.Backfill(ctx, "diario", from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 {
		t.Errorf("the second backfill created %d run(s)", again.Created)
	}
	if again.Slots != 24 || again.Existing() != 24 {
		t.Errorf("the second backfill reported %+v, want 24 slots all already there", again)
	}
}

// AND AN INTERVAL WITH NO SLOT IS A DIFFERENT ANSWER FROM ONE ALREADY DONE.
//
// This is the pair the sentence above could not tell apart: zero created
// because the work exists, and zero created because the dates name nothing.
// A daily cron at 02:00, asked for 03:00 to 04:00, fires never.
func TestABackfillOverNoSlotsSaysThereWereNone(t *testing.T) {
	s, _, _, pool := build(t, "0 2 * * *", false)
	ctx := context.Background()
	setLastSlot(t, pool, inUTC("2026-06-01T00:00:00Z"))

	empty, err := s.Backfill(ctx, "diario",
		inUTC("2026-01-01T03:00:00Z"), inUTC("2026-01-01T04:00:00Z"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Slots != 0 || empty.Created != 0 {
		t.Errorf("an interval the cron never fires in reported %+v", empty)
	}
}
