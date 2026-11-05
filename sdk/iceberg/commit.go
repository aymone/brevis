package iceberg

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"brevis.sh/brevis/sdk"
	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
)

// The commit: one snapshot per Write, the batch key in its summary, and a
// retry when another writer got there first.
//
// WHY THE DRIVER RETRIES, measured in the spike S3 (2026-11-02, two writers
// on one SQL catalog over Postgres): iceberg-go's own retry is opt-in
// (`commit.retry.num-retries`, default zero) and covers only the CAS form
// of a conflict, ErrCommitFailed, raised when the metadata pointer moved
// between read and swap. The catalogs raise most conflicts EARLIER, as a
// raw "requirement failed" from validating the staged requirements against
// the current metadata, and that form is not retried by anyone. So this
// file treats both forms as one conflict, waits with exponential backoff
// and jitter, reloads the table and redoes the append, up to CommitRetries
// attempts within CommitWithin. Created tables carry the iceberg-go retry
// turned on as well, so the CAS form is redone without rewriting files;
// the Parquet of an attempt that failed here is an orphan, and removing it
// is lake-ops' job.
//
// THE BATCH KEY IS CHECKED ON EVERY ATTEMPT, not only the first: a commit
// that landed and then failed to report -- the connection dropped after
// the catalog accepted -- looks like a conflict, and the retry that finds
// its own key in a snapshot returns already-committed instead of writing
// the batch twice. That, not Dedup, is what makes a replay land once.

const (
	// batchKeyProperty carries WriteOptions.BatchKey in the snapshot summary.
	batchKeyProperty = "brevis.batch-key"

	// writerProperty names what wrote the snapshot, for whoever reads the
	// table's history without this module at hand.
	writerProperty = "brevis.sdk"
	writerName     = "brevis sdk/iceberg"

	// strategyAlreadyCommitted is the LoadResult.Strategy of a Write whose
	// BatchKey already named a snapshot: nothing was written.
	strategyAlreadyCommitted = "already-committed"

	defaultCommitRetries = 10
	defaultCommitWithin  = 30 * time.Second
)

// commit appends the batch as one snapshot, or recognizes it as already
// there, retrying a conflicting commit as the file's comment says.
func (t Table) commit(ctx context.Context, cat catalog.Catalog, tbl *table.Table, records []sdk.Envelope, opt sdk.WriteOptions, res *sdk.LoadResult) error {
	retries, within := t.CommitRetries, t.CommitWithin
	if retries <= 0 {
		retries = defaultCommitRetries
	}
	if within <= 0 {
		within = defaultCommitWithin
	}
	deadline := time.Now().Add(within)
	props := summary(opt)

	for attempt := 1; ; attempt++ {
		if snap := committed(tbl, opt.BatchKey); snap != nil {
			alreadyCommitted(res, snap)
			return nil
		}
		schema, err := Reconcile(tbl.Schema(), records, opt)
		if err != nil {
			return err
		}
		written, err := appendOnce(ctx, tbl, schema, records, props)
		if err == nil {
			t.report(ctx, written, res, int64(len(records)))
			return nil
		}
		if !isConflict(err) {
			return fmt.Errorf("iceberg: appending to %s: %w", t.Describe(), err)
		}
		if attempt >= retries {
			return fmt.Errorf("iceberg: appending to %s: the commit conflicted with another writer on every one of %d attempts "+
				"(CommitRetries); last: %w", t.Describe(), attempt, err)
		}
		if time.Until(deadline) <= 0 {
			return fmt.Errorf("iceberg: appending to %s: the commit kept conflicting with another writer for %s "+
				"(CommitWithin), over %d attempts; last: %w", t.Describe(), within, attempt, err)
		}
		if err := wait(ctx, attempt, deadline); err != nil {
			return fmt.Errorf("iceberg: appending to %s: %w", t.Describe(), err)
		}
		tbl, err = cat.LoadTable(ctx, t.identifier())
		if err != nil {
			return fmt.Errorf("iceberg: reloading %s after a conflicting commit: %w", t.Describe(), err)
		}
	}
}

// appendOnce converts and appends the batch against one reading of the
// table. The reader is released here; iceberg-go keeps what it needs.
func appendOnce(ctx context.Context, tbl *table.Table, schema sdk.Schema, records []sdk.Envelope, props ice.Properties) (*table.Table, error) {
	rdr, err := newReader(schema, records)
	if err != nil {
		return nil, err
	}
	defer rdr.Release()
	return tbl.Append(ctx, rdr, props)
}

// summary is what this writer puts in the snapshot: who wrote it, and the
// batch key when the consumer gave one.
func summary(opt sdk.WriteOptions) ice.Properties {
	props := ice.Properties{writerProperty: writerName}
	if opt.BatchKey != "" {
		props[batchKeyProperty] = opt.BatchKey
	}
	return props
}

// committed finds the snapshot the batch key already names, if any.
// Metadata only: the snapshots and their summaries are in the file the
// table was loaded from, so this costs nothing against the catalog.
func committed(tbl *table.Table, key string) *table.Snapshot {
	if key == "" {
		return nil
	}
	for _, s := range tbl.Metadata().Snapshots() {
		if s.Summary != nil && s.Summary.Properties[batchKeyProperty] == key {
			return &s
		}
	}
	return nil
}

// alreadyCommitted fills the result for a batch that was there: the rows
// that snapshot added, and nothing written now.
func alreadyCommitted(res *sdk.LoadResult, snap *table.Snapshot) {
	res.Strategy = strategyAlreadyCommitted
	res.Objects = nil
	res.BytesStaged = 0
	if v, err := strconv.ParseInt(snap.Summary.Properties["added-records"], 10, 64); err == nil {
		res.RowsLoaded = v
	}
}

// isConflict is both forms the spike measured: the CAS form iceberg-go
// names, and the raw requirement the catalogs raise before it.
func isConflict(err error) bool {
	return errors.Is(err, table.ErrCommitFailed) || strings.Contains(err.Error(), "requirement failed")
}

// wait sleeps before the next attempt: 50 ms doubling to a 2 s cap, with
// jitter between half and one and a half of that so two writers that
// collided do not collide again on the same beat, and never past the
// deadline. A canceled context ends the wait and the Write.
func wait(ctx context.Context, attempt int, deadline time.Time) error {
	shift := min(attempt-1, 6)
	d := min(50*time.Millisecond<<shift, 2*time.Second)
	d = d/2 + time.Duration(rand.Int64N(int64(d))) // #nosec G404 -- jitter, not a secret
	if remaining := time.Until(deadline); d > remaining {
		d = remaining
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
