package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	dom "brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func changed(t *testing.T, name string, get func() (string, error), act string, do func()) {
	t.Helper()
	before, err := get()
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := get(); again != before {
		t.Fatalf("%s: fingerprint moved with no change", name)
	}
	do()
	after, err := get()
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Errorf("%s: fingerprint did not move after %s", name, act)
	}
}

func TestFingerprintsMoveWithTheirData(t *testing.T) {
	pool := insightsDB(t)
	ctx := context.Background()
	fp := postgres.NewFingerprints(pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	id := finished(t, pool, "daily", dom.StatusRunning, time.Minute)
	exec(`INSERT INTO task_runs (id, run_id, node_id, status) VALUES (gen_random_uuid(), $1, 'a', 'running')`, id)

	changed(t, "runs", func() (string, error) { return fp.Runs(ctx) }, "a new run",
		func() { finished(t, pool, "other", dom.StatusSuccess, time.Minute) })
	changed(t, "runs", func() (string, error) { return fp.Runs(ctx) }, "a run reaching a terminal state",
		func() { exec(`UPDATE runs SET status = 'success', terminado_em = now() WHERE id = $1`, id) })

	id2 := finished(t, pool, "daily", dom.StatusRunning, time.Minute)
	exec(`INSERT INTO task_runs (id, run_id, node_id, status, log) VALUES (gen_random_uuid(), $1, 'a', 'running', 'x')`, id2)
	run := func() (string, error) { return fp.Run(ctx, id2.String()) }
	changed(t, "run", run, "the log growing", func() { exec(`UPDATE task_runs SET log = log || repeat('line\n', 50) WHERE run_id = $1`, id2) })
	changed(t, "run", run, "a step finishing", func() { exec(`UPDATE task_runs SET status = 'success', terminado_em = now() WHERE run_id = $1`, id2) })
	changed(t, "run", run, "a cancel request", func() { exec(`UPDATE runs SET cancel_requested_at = now() WHERE id = $1`, id2) })
	if s, err := fp.Run(ctx, uuid.NewString()); err != nil || s != "" {
		t.Errorf("a missing run = %q, %v; want empty, nil", s, err)
	}

	wf := func() (string, error) { return fp.Workflows(ctx) }
	changed(t, "workflows", wf, "a schedule pause", func() {
		exec(`INSERT INTO schedules (id, workflow_slug, cron) VALUES (gen_random_uuid(), 'daily', '* * * * *')`)
	})
	changed(t, "workflows", wf, "a schedule pause", func() { exec(`UPDATE schedules SET ativo = false, atualizado_em = now()`) })

	one := func() (string, error) { return fp.Workflow(ctx, "daily") }
	changed(t, "workflow", one, "its run changing", func() { exec(`UPDATE runs SET attempt = attempt + 1 WHERE id = $1`, id2) })
	changed(t, "workflow", one, "its schedule changing", func() { exec(`UPDATE schedules SET ativo = true`) })
	otherBefore, _ := one()
	finished(t, pool, "unrelated", dom.StatusSuccess, time.Minute)
	if after, _ := one(); after != otherBefore {
		t.Error("workflow fingerprint moved for another workflow's run")
	}

	data := func() (string, error) { return fp.Data(ctx) }
	changed(t, "data", data, "a landing", func() {
		exec(`INSERT INTO landings (run_id, node_id, target, workflow_slug, loaded_at) VALUES ($1, 'n', 't', 'daily', now())`, id)
	})
	changed(t, "data", data, "a stream published", func() {
		exec(`INSERT INTO gateway_streams (gateway, stream_path, published_at) VALUES ('g', '/p', now())`)
	})
	changed(t, "data", data, "a stream unpublished", func() { exec(`DELETE FROM gateway_streams`) })
}

// EVERY FINGERPRINT RUNS ONCE A SECOND PER ACTIVE TOPIC, so each must stay
// under 2 ms against a table of 100k runs. The plans are asserted (no
// sequential scan of runs/task_runs/landings) and the timing is reported.
func TestFingerprintsAreCheapAt100kRuns(t *testing.T) {
	pool := insightsDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO runs (id, workflow_slug, idempotency_key, status, definicao, criado_em, iniciado_em, terminado_em)
		SELECT gen_random_uuid(), 'wf' || (i % 40), 'k' || i,
		       CASE WHEN i % 500 = 0 THEN 'running' ELSE 'success' END, '{}',
		       now() - make_interval(secs => (100000 - i) * 30),
		       now() - make_interval(secs => (100000 - i) * 30 - 1),
		       CASE WHEN i % 500 = 0 THEN NULL ELSE now() - make_interval(secs => (100000 - i) * 30 - 20) END
		FROM generate_series(1, 100000) i`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO landings (run_id, node_id, target, workflow_slug, loaded_at, recorded_at)
		SELECT id, 'n', 't' || (i % 50), workflow_slug, criado_em, criado_em
		FROM (SELECT *, row_number() OVER () i FROM runs LIMIT 100000) r`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_runs (id, run_id, node_id, status, log)
		SELECT gen_random_uuid(), id, 'a', 'success', repeat('log line\n', 20) FROM runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO schedules (id, workflow_slug, cron) SELECT gen_random_uuid(), 'wf' || i, '* * * * *' FROM generate_series(0, 39) i;
		ANALYZE runs; ANALYZE task_runs; ANALYZE landings; ANALYZE schedules`); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM runs WHERE status = 'running' LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, sql string
		args      []any
	}{
		{"runs", postgres.RunsFingerprintSQL, nil},
		{"run", postgres.RunFingerprintSQL, []any{id}},
		{"schedules", postgres.SchedulesFingerprintSQL, nil},
		{"workflow", postgres.WorkflowFingerprintSQL, []any{"wf7"}},
		{"data", postgres.DataFingerprintSQL, nil},
	} {
		best := time.Hour
		var plan string
		for i := 0; i < 5; i++ { // best of 5: the first call warms the cache
			rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, FORMAT TEXT) "+c.sql, c.args...)
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for rows.Next() {
				var l string
				_ = rows.Scan(&l)
				lines = append(lines, l)
			}
			rows.Close()
			plan = strings.Join(lines, "\n")
			var ms float64
			_, _ = fmt.Sscanf(lines[len(lines)-1], "Execution Time: %f ms", &ms)
			if d := time.Duration(ms * float64(time.Millisecond)); d < best {
				best = d
			}
		}
		t.Logf("%-9s %v\n%s", c.name, best, plan)
		for _, bad := range []string{"Seq Scan on runs", "Seq Scan on task_runs", "Seq Scan on landings"} {
			if strings.Contains(plan, bad) {
				t.Errorf("%s: plan has %q", c.name, bad)
			}
		}
		if best > 2*time.Millisecond {
			t.Errorf("%s took %v, want under 2ms", c.name, best)
		}
	}
}
