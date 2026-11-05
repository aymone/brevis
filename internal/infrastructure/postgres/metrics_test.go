package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/metrics"
)

// beforeTheMetrics is the last migration before the live metrics' two tables.
const beforeTheMetrics = 33

func metricsDB(t *testing.T) *postgres.Pool {
	t.Helper()
	pool := insightsDB(t)
	if _, err := pool.Exec(context.Background(), `TRUNCATE metric_minutes, metric_sources`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestTheMetricsMigrationGoesDownAndUp(t *testing.T) {
	pool := insightsDB(t)
	t.Cleanup(func() { migrate(t, "up") })
	has := func(table string) bool {
		var ok bool
		if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !has("metric_minutes") || !has("metric_sources") {
		t.Fatal("the tables are missing after up")
	}
	if err := postgres.MigrateDownTo(context.Background(), os.Getenv("BREVIS_TEST_DATABASE_URL"), beforeTheMetrics); err != nil {
		t.Fatal(err)
	}
	if has("metric_minutes") || has("metric_sources") {
		t.Fatal("down left a table behind")
	}
	migrate(t, "up")
	if !has("metric_minutes") || !has("metric_sources") {
		t.Fatal("up again did not rebuild them")
	}
	var comment *string
	if err := pool.QueryRow(context.Background(), `SELECT obj_description('public.metric_minutes'::regclass)`).Scan(&comment); err != nil || comment == nil {
		t.Fatalf("metric_minutes has no COMMENT: %v", err)
	}
}

func TestTheStoreRoundTripsAndTheReaderRebuildsTheSnapshot(t *testing.T) {
	pool := metricsDB(t)
	st := postgres.NewMetricsStore(pool)
	ctx := context.Background()
	now := time.Now()
	minute := now.Unix() / 60
	ex := metrics.Export{
		Sources: []metrics.SourceRow{{Kind: metrics.Gateway, Owner: "edge", Instance: "10.0.0.1:9090", Started: now.Add(-time.Hour),
			IntervalSec: 5, LastTry: now, LastOK: now, Diffs: 3}},
		Minutes: []metrics.MinuteRow{
			{Kind: metrics.Gateway, Owner: "edge", Minute: minute, Counts: make([]float64, len(metrics.GatewayMeasures)), Covered: 55},
			{Kind: metrics.Gateway, Owner: "edge", Entity: "clicks", Minute: minute,
				Counts: []float64{10, 0, 0, 0, 10, 0, 0, 0, 0}, Latency: map[string]float64{"0.5": 10, "+Inf": 10},
				Covered: 0, Gauges: []float64{4, 1}, Rate: []float64{12, 0, 0, 0, 12, 0, 0, 0, 0}},
		},
	}
	if err := st.Write(ctx, ex); err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, ex); err != nil { // the same round again is an upsert, not a duplicate
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM metric_minutes`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d minute rows after writing the same round twice, want 2", n)
	}
	snap, err := metrics.NewReader(st).Read(ctx, metrics.Gateway, 30, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Configured || len(snap.Owners) != 1 || snap.Owners[0].Replicas != 1 || snap.Owners[0].State != "ok" {
		t.Fatalf("%+v", snap.Owners)
	}
	if len(snap.Entities) != 1 || snap.Entities[0].Name != "clicks" || !snap.Entities[0].Present {
		t.Fatalf("%+v", snap.Entities)
	}
	if got := snap.Entities[0].Values["received"]; got[len(got)-1] == nil || *got[len(got)-1] != 10 {
		t.Fatalf("received %v", got)
	}
	if other, _ := metrics.NewReader(st).Read(ctx, metrics.Endpoints, 30, nil); other.Configured {
		t.Fatal("the endpoints kind has no source")
	}
	fp1, _ := st.Fingerprint(ctx, metrics.Gateway)
	time.Sleep(10 * time.Millisecond)
	_ = st.Write(ctx, ex)
	if fp2, _ := st.Fingerprint(ctx, metrics.Gateway); fp1 == fp2 {
		t.Error("the fingerprint did not move with a round")
	}
	fpE, _ := st.Fingerprint(ctx, metrics.Endpoints)
	if fpE == fp1 {
		t.Error("the endpoints fingerprint equals the gateway's")
	}
}

// A replica that is no longer announced leaves with its row: it must not stay
// as a silent replica and degrade the owner forever.
func TestAScaledDownReplicaLeavesTheSources(t *testing.T) {
	pool := metricsDB(t)
	st := postgres.NewMetricsStore(pool)
	ctx := context.Background()
	now := time.Now()
	src := func(inst string) metrics.SourceRow {
		return metrics.SourceRow{Kind: metrics.Gateway, Owner: "edge", Instance: inst, Started: now, IntervalSec: 5, LastTry: now, LastOK: now}
	}
	_ = st.Write(ctx, metrics.Export{Sources: []metrics.SourceRow{src("10.0.0.1:9090"), src("10.0.0.2:9090")}})
	snap, _ := metrics.NewReader(st).Read(ctx, metrics.Gateway, 5, nil)
	if snap.Owners[0].Replicas != 2 {
		t.Fatalf("%+v", snap.Owners)
	}
	_ = st.Write(ctx, metrics.Export{Sources: []metrics.SourceRow{src("10.0.0.1:9090")}})
	snap, _ = metrics.NewReader(st).Read(ctx, metrics.Gateway, 5, nil)
	if o := snap.Owners[0]; o.Replicas != 1 || o.State == "partial" {
		t.Fatalf("%+v", o)
	}
	// A collector that stopped: nothing was written for a while, so no source.
	if _, err := pool.Exec(ctx, `UPDATE metric_sources SET updated_at = now() - interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	if snap, _ = metrics.NewReader(st).Read(ctx, metrics.Gateway, 5, nil); snap.Configured {
		t.Fatal("a source nobody refreshes must not count as configured")
	}
}

func TestPruneRemovesOnlyTheOldAndInBatches(t *testing.T) {
	pool := metricsDB(t)
	st := postgres.NewMetricsStore(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO metric_minutes (kind, owner, entity, minute, counts)
		SELECT 'gateway', 'edge', 's' || (i % 7), 1000 + i, '[]' FROM generate_series(0, 99) i`); err != nil {
		t.Fatal(err)
	}
	// 100 rows, minutes 1000..1099. Before 1050 are 50; a batch of 20 takes 20.
	if n, err := st.Prune(ctx, 1050, 20); err != nil || n != 20 {
		t.Fatalf("first batch: %d %v", n, err)
	}
	// metrics.PruneOld drains the rest in bounded batches.
	metrics.PruneOld(ctx, st, time.Unix(1050*60+3600, 0), time.Hour, discard())
	var left, oldest int
	_ = pool.QueryRow(ctx, `SELECT count(*), min(minute) FROM metric_minutes`).Scan(&left, &oldest)
	if left != 50 || oldest != 1050 {
		t.Fatalf("%d rows left, oldest minute %d; want 50 and 1050", left, oldest)
	}
}

func TestLoweringTheRetentionPrunesALargeBacklogInBatches(t *testing.T) {
	pool := metricsDB(t)
	st := postgres.NewMetricsStore(pool)
	ctx := context.Background()
	total := metrics.PruneBatch*2 + 10
	if _, err := pool.Exec(ctx, `
		INSERT INTO metric_minutes (kind, owner, entity, minute, counts)
		SELECT 'gateway', 'edge', 's' || i, 1, '[]' FROM generate_series(1, $1) i`, total); err != nil {
		t.Fatal(err)
	}
	calls := &countingStore{Store: st}
	metrics.PruneOld(ctx, calls, time.Unix(1_000_000, 0), time.Hour, discard())
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM metric_minutes`).Scan(&left)
	if left != 0 || calls.n != 3 {
		t.Fatalf("%d rows left after %d statements; want 0 after 3 (%d rows, batches of %d)", left, calls.n, total, metrics.PruneBatch)
	}
}

type countingStore struct {
	metrics.Store
	n int
}

func (c *countingStore) Prune(ctx context.Context, before int64, limit int) (int, error) {
	c.n++
	return c.Store.Prune(ctx, before, limit)
}

// Two processes asking for the lock: exactly one runs; when it stops, the other
// takes over.
func TestTheAdvisoryLockAdmitsOneCollector(t *testing.T) {
	pool := insightsDB(t)
	var running, peak, started atomic.Int32
	run := func(ctx context.Context) {
		n := running.Add(1)
		started.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-ctx.Done()
		running.Add(-1)
	}
	ctxA, stopA := context.WithCancel(context.Background())
	ctxB, stopB := context.WithCancel(context.Background())
	defer stopB()
	var wg sync.WaitGroup
	for _, c := range []context.Context{ctxA, ctxB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.RunExclusive(c, postgres.MetricsLockKey, 50*time.Millisecond, discard(), run)
		}()
	}
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			if cond() {
				return
			}
		}
		t.Fatalf("timed out: %s", what)
	}
	waitFor("one collector started", func() bool { return started.Load() == 1 })
	time.Sleep(300 * time.Millisecond)
	if running.Load() != 1 || started.Load() != 1 {
		t.Fatalf("running %d, started %d: the second process must wait", running.Load(), started.Load())
	}
	// Whichever won stops (cancel both would stop both): cancel A; if A held it,
	// B takes over, otherwise B was the holder and A was never admitted.
	stopA()
	waitFor("a takeover or a clean stop", func() bool { return started.Load() == 2 || running.Load() == 1 })
	stopB()
	wg.Wait()
	if peak.Load() > 1 {
		t.Fatalf("%d collectors ran at once", peak.Load())
	}
}
