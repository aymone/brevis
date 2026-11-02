package iceberg

import (
	"fmt"

	"github.com/AreteAcademy/brevis/sdk"
)

// check refuses a Table that cannot name itself, before anything else: the
// three segments of iceberg://{catalog}/{namespace}/{table} are what the
// engine's `landed` line, the console and the cache key on.
func (t Table) check() error {
	if t.Catalog.Name == "" {
		return fmt.Errorf("iceberg.Table needs Catalog.Name: it is the first segment of iceberg://{catalog}/{namespace}/{table}")
	}
	if t.Namespace == "" || t.Name == "" {
		return fmt.Errorf("iceberg.Table needs Namespace and Name: `bronze` and `events` in iceberg://lake/bronze/events")
	}
	return nil
}

// refuse is every option this destination will not take, said before a
// catalog is opened. A flag that is accepted and does nothing is the defect
// this SDK has found most often in itself, so each of these names the option
// and what to do instead.
//
// DEDUP IS REFUSED AND NOT DEFERRED: Iceberg has no key to match on at
// append time. The honest implementation is an equality delete on
// ingestion_id plus a row delta, and it arrives when DuckDB reads those
// deletes reliably -- not as a flag that writes a plain append and calls it
// a merge.
//
// Evolve is not here because it is not an option: in this SDK it is a field
// of the destinations that evolve, and this one has no such field. A
// divergence between the batch and the table is Reconcile's error, never an
// ALTER.
func (t Table) refuse(opt sdk.WriteOptions) error {
	// Dedup is a string, and its zero value is "not set": only none passes.
	if opt.Dedup != "" && opt.Dedup != sdk.DedupNone {
		return fmt.Errorf("iceberg.Table cannot deduplicate yet: v1 appends. " +
			"Drop Dedup and rely on BatchKey, which makes a retried batch land once")
	}
	for _, c := range opt.Schema {
		// Iceberg v2 has no server to fill a default and no index to hold a
		// unique: the column would land NULL and call it a default, or land
		// twice and call it unique. Both are the silent kind of wrong.
		if c.Default != nil {
			return fmt.Errorf("iceberg.Table cannot fill a Default: column %q declares one, and an Iceberg table has nothing "+
				"that applies it, so the row would land NULL. Drop Default and compose the value in Transform", c.Name)
		}
		if c.Unique {
			return fmt.Errorf("iceberg.Table cannot enforce Unique: column %q declares it, and an Iceberg table has no unique "+
				"index. Drop Unique and rely on BatchKey, which makes a retried batch land once", c.Name)
		}
	}
	if t.PartitionBy != "" && opt.PartitionBy != "" && t.PartitionBy != opt.PartitionBy {
		return fmt.Errorf("iceberg.Table has PartitionBy %q and the Target has PartitionBy %q; one column partitions a table, "+
			"so set it in one place", t.PartitionBy, opt.PartitionBy)
	}
	return nil
}
