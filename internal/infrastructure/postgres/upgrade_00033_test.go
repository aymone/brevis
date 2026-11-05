package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	dom "brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// MIGRATION 00033 ON A DATABASE THAT ALREADY HOLDS DATA.
//
// 00033 renames the table, columns, indexes, constraint and sequence that were
// Portuguese. A rename keeps the rows, and this is what says so: a database is
// built with the migrations as they were BEFORE 00033 (testdata/before00033,
// a frozen copy), filled with a row in every table that is renamed, and
// upgraded with the real migration. Then the rows are read under the new names,
// the engine's own repositories read what the old release wrote (stage keys in
// Portuguese included), the comments are English, Down puts every old name
// back, and a database upgraded this way has the same schema as one that was
// built from scratch.

// scratchDatabase makes an empty database on the test server and returns its
// URL. It is dropped when the test ends.
func scratchDatabase(t *testing.T, name string) string {
	t.Helper()
	base := os.Getenv("BREVIS_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set BREVIS_TEST_DATABASE_URL to run the upgrade test (make up)")
	}
	name = strings.ReplaceAll(name+"_"+uuid.NewString()[:8], "-", "_")
	admin, err := pgx.Connect(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(context.Background(), `CREATE DATABASE `+name); err != nil {
		t.Skipf("cannot create a scratch database here: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), base)
		if err != nil {
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		_, _ = c.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// migrateBefore00033 applies the migrations as they were before 00033.
func migrateBefore00033(t *testing.T, dsn string) {
	t.Helper()
	goose.SetBaseFS(os.DirFS("testdata/before00033"))
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(context.Background(), openSQL(t, dsn), "."); err != nil {
		t.Fatalf("migrating a database to the state before 00033: %v", err)
	}
}

// seedBefore00033 writes one row in every table 00033 renames, with the
// Portuguese names and the Portuguese JSON keys the old release wrote.
func seedBefore00033(t *testing.T, db *sql.DB) (runID uuid.UUID) {
	t.Helper()
	runID = uuid.New()
	project, wfID := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO projects (id, slug, name) VALUES ($1, 'p', 'p')`, []any{project}},
		{`INSERT INTO workflows (id, project_id, slug, name, definicao) VALUES ($1, $2, 'daily', 'Daily', $3)`,
			[]any{wfID, project, `{"Slug":"daily","Name":"Daily","Kind":"chain","Params":[{"Nome":"date","Tipo":"string","Default":"","Descricao":"the day","Enum":null,"Pattern":""}],"Nodes":[{"ID":"load","Run":"echo hi"}]}`}},
		{`INSERT INTO schedules (workflow_slug, id, cron, timezone, catchup, ativo, ultimo_slot)
		  VALUES ('daily', gen_random_uuid(), '0 4 * * *', 'UTC', false, false, '2026-10-01T04:00:00Z')`, nil},
		{`INSERT INTO runs (id, workflow_slug, idempotency_key, status, attempt, definicao, erro, criado_em, iniciado_em, terminado_em, max_ativos, grupo_concorrencia)
		  VALUES ($1, 'daily', 'k1', 'failed', 2, '{"Slug":"daily"}', 'boom', '2026-10-01T04:00:00Z', '2026-10-01T04:00:05Z', '2026-10-01T04:01:00Z', 3, 'grp')`, []any{runID}},
		{`INSERT INTO task_runs (id, run_id, node_id, status, attempt, erro, iniciado_em, terminado_em, etapas, sdk_versao, saida, log)
		  VALUES (gen_random_uuid(), $1, 'load', 'failed', 1, 'step boom', '2026-10-01T04:00:06Z', '2026-10-01T04:00:50Z',
		  '[{"indice":0,"nome":"extract","estado":"done","ms":31000,"em":"x","numeros":{"pages":12,"bytes":5000000}},{"indice":1,"nome":"load","estado":"done","ms":22000,"em":"x","numeros":{"rows":48213,"records":48300}}]',
		  'v0.47.0', '{"k":"v"}', 'log line')`, []any{runID}},
		{`INSERT INTO queue_items (run_id, prioridade, disponivel_em, reivindicado_em, reivindicado_por, criado_em, renovado_em)
		  VALUES ($1, 7, '2026-10-01T04:00:00Z', '2026-10-01T04:00:02Z', 'worker-1', '2026-10-01T03:59:00Z', '2026-10-01T04:00:30Z')`, []any{runID}},
		{`INSERT INTO alertas (run_id, tipo, node_id, canal, payload, disponivel_em, reivindicado_em, reivindicado_por, tentativas, erro, entregue_em, desistiu_em, criado_em)
		  VALUES ($1, 'step', 'load', 'SLACK', '{"text":"x"}', '2026-10-01T04:01:00Z', '2026-10-01T04:01:01Z', 'alerter', 4, '403 from Slack', NULL, '2026-10-01T04:05:00Z', '2026-10-01T04:01:00Z')`, []any{runID}},
		{`INSERT INTO load_metrics (run_id, node_id, map_index, workflow_slug, em, linhas, registros, ignorados, bytes_saida, load_ms, bytes_entrada, paginas, tentativas, extract_ms)
		  VALUES ($1, 'load', -1, 'daily', '2026-10-01T04:00:00Z', 48213, 48300, 87, 9000, 22000, 5000000, 12, 3, 31000)`, []any{runID}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seeding the old schema: %v\n%s", err, q.sql)
		}
	}
	return runID
}

// schemaOf is everything 00033 touches, as text a comparison can read: the
// columns, the indexes, the constraints, the sequences and the comments.
func schemaOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '')
		   FROM information_schema.columns WHERE table_schema = 'public' AND table_name <> 'goose_db_version' ORDER BY 1`,
		`SELECT 'index ' || indexname || ' ' || indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename <> 'goose_db_version' ORDER BY 1`,
		`SELECT 'constraint ' || conrelid::regclass::text || ' ' || conname FROM pg_constraint
		   WHERE connamespace = 'public'::regnamespace AND conrelid::regclass::text <> 'goose_db_version' ORDER BY 1`,
		`SELECT 'sequence ' || sequencename FROM pg_sequences WHERE schemaname = 'public' AND sequencename <> 'goose_db_version_id_seq' ORDER BY 1`,
		`SELECT 'comment ' || c.relname || '.' || COALESCE(a.attname, '') || ' ' || d.description
		   FROM pg_description d JOIN pg_class c ON c.oid = d.objoid
		   LEFT JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = d.objsubid AND d.objsubid > 0
		   WHERE c.relnamespace = 'public'::regnamespace ORDER BY 1`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("reading the schema: %v", err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		_ = rows.Close()
	}
	return out
}

func TestMigration00033KeepsTheRowsOfAnExistingDatabase(t *testing.T) {
	ctx := context.Background()
	dsn := scratchDatabase(t, "brevis_upgrade")
	db := openSQL(t, dsn)
	migrateBefore00033(t, dsn)
	runID := seedBefore00033(t, db)
	before := schemaOf(t, db)

	// The release's own binary path: the embedded migrations, from 32 to 33.
	if err := postgres.Migrate(ctx, dsn, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	t.Run("the rows are intact under the new names", func(t *testing.T) {
		for _, c := range []struct {
			name, query string
			want        []any
		}{
			{"runs", `SELECT definition::text, error, created_at = '2026-10-01T04:00:00Z', started_at IS NOT NULL, finished_at = '2026-10-01T04:01:00Z', max_active, concurrency_group FROM runs WHERE id = $1`,
				[]any{`{"Slug": "daily"}`, "boom", true, true, true, int64(3), "grp"}},
			{"task_runs", `SELECT error, started_at IS NOT NULL, finished_at IS NOT NULL, jsonb_array_length(stages)::int8, sdk_version, output::text FROM task_runs WHERE run_id = $1`,
				[]any{"step boom", true, true, int64(2), "v0.47.0", `{"k": "v"}`}},
			{"queue_items", `SELECT priority, claimed_by, claimed_at IS NOT NULL, available_at IS NOT NULL, created_at IS NOT NULL, renewed_at = '2026-10-01T04:00:30Z' FROM queue_items WHERE run_id = $1`,
				[]any{int64(7), "worker-1", true, true, true, true}},
			{"alerts", `SELECT kind, node_id, channel, attempts, error, delivered_at IS NULL, gave_up_at IS NOT NULL, claimed_by, created_at IS NOT NULL FROM alerts WHERE run_id = $1`,
				[]any{"step", "load", "SLACK", int64(4), "403 from Slack", true, true, "alerter", true}},
			{"load_metrics", `SELECT rows, records, ignored, bytes_out, bytes_in, pages, attempts, run_created_at = '2026-10-01T04:00:00Z' FROM load_metrics WHERE run_id = $1`,
				[]any{int64(48213), int64(48300), int64(87), int64(9000), int64(5000000), int64(12), int64(3), true}},
		} {
			dest := make([]any, len(c.want))
			ptrs := make([]any, len(c.want))
			for i := range dest {
				ptrs[i] = &dest[i]
			}
			if err := db.QueryRow(c.query, runID).Scan(ptrs...); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if !reflect.DeepEqual(dest, c.want) {
				t.Errorf("%s: read %v, wanted %v", c.name, dest, c.want)
			}
		}
		var active bool
		var last any
		if err := db.QueryRow(`SELECT active, last_slot FROM schedules WHERE workflow_slug = 'daily'`).Scan(&active, &last); err != nil || active || last == nil {
			t.Errorf("schedules: active=%v last_slot=%v err=%v", active, last, err)
		}
		var def string
		if err := db.QueryRow(`SELECT definition::text FROM workflows WHERE slug = 'daily'`).Scan(&def); err != nil || !strings.Contains(def, `"Nome": "date"`) {
			t.Errorf("workflows.definition = %s (%v)", def, err)
		}
		// The old names are gone, and the sequence went with its table.
		for _, gone := range []string{`SELECT 1 FROM alertas`, `SELECT criado_em FROM runs`, `SELECT etapas FROM task_runs`} {
			if _, err := db.Exec(gone); err == nil {
				t.Errorf("%q still works after the migration", gone)
			}
		}
		if _, err := db.Exec(`INSERT INTO alerts (run_id, kind, channel, payload) VALUES ($1, 'run', 'SLACK', '{}')`, runID); err != nil {
			t.Errorf("a new alert: %v", err)
		}
		var id int64
		if err := db.QueryRow(`SELECT max(id) FROM alerts`).Scan(&id); err != nil || id != 2 {
			t.Errorf("the alerts sequence continued at %d (%v), want 2", id, err)
		}
	})

	t.Run("the engine reads what the old release wrote", func(t *testing.T) {
		pool, err := postgres.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		runs := postgres.NewRunRepo(pool)

		run, err := runs.Get(ctx, runID)
		if err != nil || run.Err != "boom" || run.MaxActive != 3 {
			t.Fatalf("Get = %+v, %v", run, err)
		}
		states, err := postgres.NewRunRepo(pool).NodeStates(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		load := states["load"]
		if len(load.Stages) != 2 || load.Stages[0].Name != "extract" || load.Stages[1].State != "done" ||
			load.Stages[1].Numbers["rows"] != 48213.0 || load.SdkVersion != "v0.47.0" {
			t.Errorf("a step recorded with Portuguese stage keys reads back as %+v", load)
		}
		latest, err := postgres.NewReadRepo(pool).LatestRuns(ctx, 5)
		if err != nil || len(latest) != 1 || latest[0].ID != runID.String() {
			t.Errorf("LatestRuns = %+v, %v", latest, err)
		}
		if _, err := postgres.NewReadRepo(pool).LoadTrend(ctx, "daily", 30); err != nil {
			t.Errorf("LoadTrend: %v", err)
		}

		// And it can keep going: a workflow published and a run created.
		wfs := postgres.NewWorkflowRepo(pool)
		var project uuid.UUID
		if err := db.QueryRow(`SELECT id FROM projects`).Scan(&project); err != nil {
			t.Fatal(err)
		}
		w := daily("0 5 * * *", "echo hi")
		w.Slug = "fresh"
		if err := wfs.Publish(ctx, w, project, "cli"); err != nil {
			t.Fatalf("publish: %v", err)
		}
		made, err := runs.Create(ctx, dom.Run{WorkflowSlug: "fresh", IdempotencyKey: uuid.NewString(), Definition: []byte(`{}`)})
		if err != nil {
			t.Fatalf("create a run: %v", err)
		}
		if got, err := runs.Get(ctx, made.ID); err != nil || got.WorkflowSlug != "fresh" {
			t.Errorf("the new run reads back as %+v, %v", got, err)
		}
	})

	t.Run("the comments are English", func(t *testing.T) {
		for _, s := range schemaOf(t, db) {
			if !strings.HasPrefix(s, "comment ") {
				continue
			}
			for _, pt := range []string{"Caixa", "Numeros", "Execucoes", "Indice", "Parametros", "Grupo de", "Leituras", "Ultima", "Onde o step"} {
				if strings.Contains(s, pt) {
					t.Errorf("a Portuguese comment survived: %s", s)
				}
			}
		}
	})

	t.Run("down puts every old name back", func(t *testing.T) {
		if err := postgres.MigrateDownTo(ctx, dsn, 32); err != nil {
			t.Fatalf("migrate down: %v", err)
		}
		// The Portuguese comments are not restored (the old files carry English
		// ones now); everything else must be what it was.
		if got, want := without(schemaOf(t, db), "comment "), without(before, "comment "); !reflect.DeepEqual(got, want) {
			t.Errorf("after down the schema differs from the one before 00033:\n%s", diffLines(want, got))
		}
		if err := postgres.Migrate(ctx, dsn, "up"); err != nil {
			t.Fatalf("migrate up again: %v", err)
		}
	})

	t.Run("an upgraded database equals a fresh one", func(t *testing.T) {
		freshDSN := scratchDatabase(t, "brevis_fresh")
		if err := postgres.Migrate(ctx, freshDSN, "up"); err != nil {
			t.Fatalf("a fresh install: %v", err)
		}
		if got, want := schemaOf(t, db), schemaOf(t, openSQL(t, freshDSN)); !reflect.DeepEqual(got, want) {
			t.Errorf("the upgraded schema differs from a fresh install:\n%s", diffLines(want, got))
		}
	})
}

func without(lines []string, prefix string) []string {
	var out []string
	for _, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

func diffLines(want, got []string) string {
	in := func(set []string, s string) bool {
		for _, x := range set {
			if x == s {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	for _, l := range want {
		if !in(got, l) {
			fmt.Fprintf(&b, "  missing: %s\n", l)
		}
	}
	for _, l := range got {
		if !in(want, l) {
			fmt.Fprintf(&b, "  extra:   %s\n", l)
		}
	}
	return b.String()
}
