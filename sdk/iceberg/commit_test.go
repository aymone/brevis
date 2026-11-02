package iceberg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"brevis.sh/brevis/sdk"
	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	iceio "github.com/apache/iceberg-go/io"
	"github.com/apache/iceberg-go/table"
)

// flaky is the disk catalog with a CommitTable that fails the way a busy
// catalog does: the first `failures` commits return `fail`, the rest go
// through. With `commitAnyway` the inner commit happens before the error
// is returned, which is the network blip after a commit that already
// landed -- the case a retry must not turn into a duplicate. LoadTable
// hands back a table bound to the wrapper, so that commits route through
// it.
type flaky struct {
	catalog.Catalog
	failures     int32
	fail         error
	commitAnyway bool
	commits      atomic.Int32
}

func (f *flaky) LoadTable(ctx context.Context, ident table.Identifier) (*table.Table, error) {
	inner, err := f.Catalog.LoadTable(ctx, ident)
	if err != nil {
		return nil, err
	}
	fsF := func(ctx context.Context) (iceio.IO, error) { return inner.FS(ctx) }
	return table.New(inner.Identifier(), inner.Metadata(), inner.MetadataLocation(), fsF, f), nil
}

// CreateTable rebinds too: a table that came out of the inner catalog's
// CreateTable would commit through the inner catalog, past this wrapper.
func (f *flaky) CreateTable(ctx context.Context, ident table.Identifier, sc *ice.Schema, opts ...catalog.CreateTableOpt) (*table.Table, error) {
	if _, err := f.Catalog.CreateTable(ctx, ident, sc, opts...); err != nil {
		return nil, err
	}
	return f.LoadTable(ctx, ident)
}

func (f *flaky) CommitTable(ctx context.Context, ident table.Identifier, reqs []table.Requirement, updates []table.Update) (table.Metadata, string, error) {
	n := f.commits.Add(1)
	if n <= f.failures {
		if f.commitAnyway {
			if _, _, err := f.Catalog.CommitTable(ctx, ident, reqs, updates); err != nil {
				return nil, "", err
			}
		}
		return nil, "", f.fail
	}
	return f.Catalog.CommitTable(ctx, ident, reqs, updates)
}

func withFlaky(t *testing.T, failures int, fail error, commitAnyway bool) (Table, *flaky) {
	t.Helper()
	lake := onDisk(t)
	f := &flaky{Catalog: lake.Conn, failures: int32(failures), fail: fail, commitAnyway: commitAnyway}
	tbl := Table{Catalog: Catalog{Name: "lake", Conn: f}, Namespace: "bronze", Name: "events", CreateTable: yes()}
	return tbl, f
}

func snapshotsOf(t *testing.T, tbl Table) []table.Snapshot {
	t.Helper()
	got, err := tbl.Catalog.Conn.LoadTable(context.Background(), catalog.ToIdentifier("bronze", "events"))
	if err != nil {
		t.Fatal(err)
	}
	return got.Metadata().Snapshots()
}

// THE BATCH KEY GOES INTO THE SNAPSHOT'S SUMMARY, AND THE SAME KEY TWICE IS
// ONE SNAPSHOT (criterion 2): the second Write writes nothing, reports the
// rows the first one added, Strategy "already-committed" and no Objects.
// It is what the gateway needs for a dead-letter replay not to duplicate,
// since Dedup is refused. A different key appends as usual.
func TestTheSameBatchKeyTwiceIsOneSnapshot(t *testing.T) {
	ctx := context.Background()
	tbl := Table{Catalog: onDisk(t), Namespace: "bronze", Name: "events", CreateTable: yes()}
	opt := sdk.WriteOptions{Columns: events.Names(), Schema: events, BatchKey: "2026-11-02T10:00:00Z/1"}

	first, err := tbl.Write(ctx, eventRows(3, 1), opt)
	if err != nil {
		t.Fatal(err)
	}
	snaps := snapshotsOf(t, tbl)
	if len(snaps) != 1 {
		t.Fatalf("%d snapshots after the first write", len(snaps))
	}
	if got := snaps[0].Summary.Properties["brevis.batch-key"]; got != opt.BatchKey {
		t.Errorf("summary brevis.batch-key = %q, want %q", got, opt.BatchKey)
	}
	if got := snaps[0].Summary.Properties["brevis.sdk"]; got == "" {
		t.Errorf("summary has no brevis.sdk: %v", snaps[0].Summary.Properties)
	}

	second, err := tbl.Write(ctx, eventRows(5, 100), opt) // other rows, same key: a replay
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Strategy != "already-committed" || second.RowsLoaded != first.RowsLoaded || len(second.Objects) != 0 || second.BytesStaged != 0 {
		t.Errorf("replay result %+v, want already-committed with the first write's %d rows and nothing written", second, first.RowsLoaded)
	}
	if n := len(snapshotsOf(t, tbl)); n != 1 {
		t.Errorf("%d snapshots after the replay, want still 1", n)
	}

	opt.BatchKey = "2026-11-02T10:00:00Z/2"
	third, err := tbl.Write(ctx, eventRows(2, 200), opt)
	if err != nil {
		t.Fatal(err)
	}
	if third.Strategy != "append" || third.RowsLoaded != 2 || len(snapshotsOf(t, tbl)) != 2 {
		t.Errorf("another key: %+v, want a second snapshot", third)
	}
}

// A CONFLICT IS RETRIED: THE TABLE IS RELOADED AND THE APPEND REDONE, with
// backoff and jitter, in both forms the spike S3 measured -- the raw
// "requirement failed" the SQL catalog raises before the CAS, which
// iceberg-go does not retry, and ErrCommitFailed, which it does when the
// table says so. Either way the rows land once.
func TestAConflictIsRetriedUntilTheCommitLands(t *testing.T) {
	for name, fail := range map[string]error{
		"requirement failed": errors.New(`requirement failed: branch "main" has changed: expected id 1, found 2`),
		"ErrCommitFailed":    fmt.Errorf("%w: another writer", table.ErrCommitFailed),
	} {
		t.Run(name, func(t *testing.T) {
			tbl, f := withFlaky(t, 2, fail, false)
			tbl.CommitRetries, tbl.CommitWithin = 5, 10*time.Second
			start := time.Now()
			res, err := tbl.Write(context.Background(), eventRows(4, 1), sdk.WriteOptions{Columns: events.Names(), Schema: events, BatchKey: "k"})
			if err != nil {
				t.Fatalf("Write after two conflicts: %v", err)
			}
			if res.RowsLoaded != 4 || res.Strategy != "append" {
				t.Errorf("result %+v", res)
			}
			if n := f.commits.Load(); n != 3 {
				t.Errorf("%d commits, want two refused and one landed", n)
			}
			if snaps := snapshotsOf(t, tbl); len(snaps) != 1 || snaps[0].Summary.Properties["total-records"] != "4" {
				t.Errorf("%d snapshots; want one with 4 records", len(snaps))
			}
			if time.Since(start) < 50*time.Millisecond {
				t.Errorf("two retries took %s; the backoff did not wait", time.Since(start))
			}
		})
	}
}

// A COMMIT THAT LANDED AND THEN FAILED TO REPORT IS NOT WRITTEN TWICE: the
// retry reloads the table, finds its own batch key in a snapshot, and
// returns already-committed. This is the network blip after the commit,
// and the reason the key is checked on every attempt and not only on the
// first.
func TestARetryThatFindsItsOwnKeyDoesNotWriteTwice(t *testing.T) {
	tbl, f := withFlaky(t, 1, errors.New("requirement failed: the connection dropped after the commit"), true)
	res, err := tbl.Write(context.Background(), eventRows(3, 1), sdk.WriteOptions{Columns: events.Names(), Schema: events, BatchKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Strategy != "already-committed" || res.RowsLoaded != 3 {
		t.Errorf("result %+v, want already-committed with the 3 rows that landed", res)
	}
	if n := f.commits.Load(); n != 1 {
		t.Errorf("%d commits, want the one that landed and no second", n)
	}
	if n := len(snapshotsOf(t, tbl)); n != 1 {
		t.Errorf("%d snapshots, want 1", n)
	}
}

// THE RETRY STOPS AT THE ATTEMPTS OR AT THE TIME CEILING, NAMING BOTH, and
// does not retry what is not a conflict: a refusal from the catalog is a
// refusal, and retrying it ten times would hide it for thirty seconds.
func TestTheRetryHasACeilingAndRetriesOnlyConflicts(t *testing.T) {
	conflict := errors.New("requirement failed: busy")
	tbl, f := withFlaky(t, 100, conflict, false)
	tbl.CommitRetries, tbl.CommitWithin = 3, 10*time.Second
	_, err := tbl.Write(context.Background(), eventRows(1, 1), sdk.WriteOptions{Columns: events.Names(), Schema: events})
	if err == nil || !strings.Contains(err.Error(), "3 attempts") || !strings.Contains(err.Error(), "lake.bronze.events") {
		t.Errorf("after the attempts: %v, want a failure naming them and the table", err)
	}
	if n := f.commits.Load(); n != 3 {
		t.Errorf("%d commits, want exactly the 3 attempts", n)
	}

	tbl, f = withFlaky(t, 100, conflict, false)
	tbl.CommitRetries, tbl.CommitWithin = 100, 150*time.Millisecond
	_, err = tbl.Write(context.Background(), eventRows(1, 1), sdk.WriteOptions{Columns: events.Names(), Schema: events})
	if err == nil || !strings.Contains(err.Error(), "150ms") {
		t.Errorf("after the ceiling: %v, want a failure naming it", err)
	}
	if n := f.commits.Load(); n >= 100 {
		t.Errorf("%d commits in 150ms; the ceiling did not stop it", n)
	}

	tbl, f = withFlaky(t, 100, errors.New("permission denied on the metadata path"), false)
	_, err = tbl.Write(context.Background(), eventRows(1, 1), sdk.WriteOptions{Columns: events.Names(), Schema: events})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("a non-conflict: %v, want it reported as it came", err)
	}
	if n := f.commits.Load(); n != 1 {
		t.Errorf("%d commits for a non-conflict, want 1", n)
	}
}

// TWO WRITERS AT ONCE ON ONE TABLE, AND EVERY BATCH LANDS (criterion 6): no
// snapshot lost in silence, the record count the sum of the batches, each
// writer's keys all present. Here against the disk catalog, which raises
// both forms of conflict like the SQL one; the service version is D8's.
func TestTwoConcurrentWritersLandEveryBatch(t *testing.T) {
	const writers, each, rows = 2, 5, 10
	lake := onDisk(t)
	ctx := context.Background()
	if _, err := (Table{Catalog: lake, Namespace: "bronze", Name: "events", CreateTable: yes()}).Write(ctx, nil, sdk.WriteOptions{Columns: events.Names(), Schema: events}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, writers*each)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			tbl := Table{Catalog: lake, Namespace: "bronze", Name: "events"}
			for i := 0; i < each; i++ {
				opt := sdk.WriteOptions{Columns: events.Names(), Schema: events, BatchKey: fmt.Sprintf("w%d/%d", w, i)}
				if _, err := tbl.Write(ctx, eventRows(rows, w*1000+i*rows), opt); err != nil {
					errs <- fmt.Errorf("writer %d batch %d: %w", w, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	snaps := snapshotsOf(t, Table{Catalog: lake, Namespace: "bronze", Name: "events"})
	keys := map[string]bool{}
	for _, s := range snaps {
		keys[s.Summary.Properties["brevis.batch-key"]] = true
	}
	if len(snaps) != writers*each || len(keys) != writers*each {
		t.Errorf("%d snapshots with %d distinct keys, want %d", len(snaps), len(keys), writers*each)
	}
	if total := snaps[len(snaps)-1].Summary.Properties["total-records"]; total != fmt.Sprint(writers*each*rows) {
		t.Errorf("total-records %q, want %d", total, writers*each*rows)
	}
}
