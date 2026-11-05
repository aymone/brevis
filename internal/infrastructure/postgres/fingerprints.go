package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Fingerprints answers, for the live-update stream, "has this screen's data
// changed?" in one cheap query per topic. Each result is an opaque string:
// equal means nothing to refetch. They run once a second per ACTIVE topic,
// shared by every browser, so each must stay index-backed and far under 2 ms
// at 100k runs (measured in fingerprints_test.go).
//
// A fingerprint may be wrong in the safe direction (change with nothing new on
// screen: one needless refetch) but must never miss a change the screen shows.
type Fingerprints struct{ pool *Pool }

func NewFingerprints(pool *Pool) *Fingerprints { return &Fingerprints{pool: pool} }

// Both statements are exported for the test to EXPLAIN them.
const (
	// runsFingerprintSQL: the three max() calls are each one step down an
	// index (criado_em, iniciado_em partial, terminado_em partial), and the
	// count walks only runs_status_idx, which holds the non-terminal runs --
	// a handful. Together they move on: a new run (criado_em), a start
	// (iniciado_em), a finish or a retry's end (terminado_em), and a run
	// entering or leaving a live state (the count).
	runsFingerprintSQL = `
		SELECT concat_ws('|',
		  (SELECT max(criado_em)    FROM runs),
		  (SELECT max(iniciado_em)  FROM runs),
		  (SELECT max(terminado_em) FROM runs),
		  (SELECT count(*) FROM runs WHERE status NOT IN ('success', 'canceled')))`

	// runFingerprintSQL: the run's own row plus its steps. pg_column_size(log)
	// and not length(log): length() has to detoast the whole log, and a
	// finished step's log can be megabytes; the stored size is read from the
	// TOAST pointer for free and still grows with the log (compressed size
	// moving to the exact same byte count is the only miss, and the very next
	// line moves it again).
	runFingerprintSQL = `
		SELECT concat_ws('|', r.status, r.attempt, r.iniciado_em, r.terminado_em, r.cancel_requested_at,
		  (SELECT concat_ws('/', count(*), coalesce(sum(pg_column_size(t.log)), 0), max(t.terminado_em),
		            string_agg(t.node_id || ':' || t.map_index || ':' || t.attempt || ':' || t.status, ',' ORDER BY t.node_id, t.map_index, t.attempt))
		   FROM task_runs t WHERE t.run_id = r.id))
		FROM runs r WHERE r.id = $1`

	// schedulesFingerprintSQL: a handful of rows, so a scan is cheaper than
	// any index. Pausing, resuming or editing a schedule moves it.
	schedulesFingerprintSQL = `
		SELECT concat_ws('|', count(*) FILTER (WHERE ativo), max(atualizado_em)) FROM schedules`

	// workflowFingerprintSQL: max(iniciado_em) per workflow has no index, so
	// the fingerprint is the newest 50 runs of the slug (runs_workflow_criado_idx,
	// one backward range scan) hashed with the fields the screen shows, plus
	// the slug's schedule (UNIQUE on workflow_slug). A run older than the 50
	// newest that changes is not on the workflow's first screen.
	workflowFingerprintSQL = `
		SELECT concat_ws('|',
		  (SELECT md5(coalesce(string_agg(concat_ws(',', id, status, attempt, iniciado_em, terminado_em), ';'), ''))
		   FROM (SELECT id, status, attempt, iniciado_em, terminado_em FROM runs
		         WHERE workflow_slug = $1 ORDER BY criado_em DESC LIMIT 50) x),
		  (SELECT concat_ws(',', ativo, atualizado_em) FROM schedules WHERE workflow_slug = $1))`

	// dataFingerprintSQL, for /data (catalog) and /ingest:
	//  - max(landings.recorded_at): every landing, write or retry, stamps it
	//    (landings_recorded_idx, one step); it is the engine's clock, whereas
	//    loaded_at is the step's own and not monotonic.
	//  - landings in the last hour: the "recent loads" shown age out of the
	//    window without any write, which no max() can see (same index, a range).
	//  - gateway_streams: published_at plus count, because unpublishing a
	//    stream deletes a row and moves no max(). The table has one row per
	//    stream, tens.
	//  - load_metrics is NOT read: it has no index on em alone (a max() would
	//    scan the table) and a row is written in the same event as the landing
	//    for the same step, so recorded_at moves with it.
	dataFingerprintSQL = `
		SELECT concat_ws('|',
		  (SELECT max(recorded_at) FROM landings),
		  (SELECT count(*) FROM landings WHERE recorded_at > now() - interval '1 hour'),
		  (SELECT max(published_at) FROM gateway_streams),
		  (SELECT count(*) FROM gateway_streams))`
)

// Query exports for the test that EXPLAINs them.
const (
	RunsFingerprintSQL      = runsFingerprintSQL
	RunFingerprintSQL       = runFingerprintSQL
	WorkflowFingerprintSQL  = workflowFingerprintSQL
	DataFingerprintSQL      = dataFingerprintSQL
	SchedulesFingerprintSQL = schedulesFingerprintSQL
)

func (f *Fingerprints) one(ctx context.Context, sql string, args ...any) (string, error) {
	var s *string
	err := f.pool.QueryRow(ctx, sql, args...).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", nil
	}
	return *s, nil
}

// Runs is the fingerprint of the runs table (topics runs, nav, overview).
func (f *Fingerprints) Runs(ctx context.Context) (string, error) {
	return f.one(ctx, runsFingerprintSQL)
}

// Run is one run and its steps (topic run:<id>). A missing run is a stable
// empty fingerprint, not an error: it may be created or pruned later.
func (f *Fingerprints) Run(ctx context.Context, id string) (string, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("run id: %w", err)
	}
	return f.one(ctx, runFingerprintSQL, u)
}

// Workflows is the runs fingerprint plus the schedules (topic workflows).
func (f *Fingerprints) Workflows(ctx context.Context) (string, error) {
	r, err := f.Runs(ctx)
	if err != nil {
		return "", err
	}
	s, err := f.one(ctx, schedulesFingerprintSQL)
	return r + "#" + s, err
}

// Workflow is one workflow's recent runs and schedule (topic workflow:<slug>).
func (f *Fingerprints) Workflow(ctx context.Context, slug string) (string, error) {
	return f.one(ctx, workflowFingerprintSQL, slug)
}

// Data is the catalog and ingest screens' fingerprint (topics catalog, ingest).
func (f *Fingerprints) Data(ctx context.Context) (string, error) {
	return f.one(ctx, dataFingerprintSQL)
}
