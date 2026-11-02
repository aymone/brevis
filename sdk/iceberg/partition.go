package iceberg

import (
	"fmt"
	"strings"

	ice "github.com/apache/iceberg-go"
)

// partitionSpec is day(column) on the schema, or no partitioning when there
// is no column. Pure, like the schemas: it takes the Iceberg schema because
// a partition field names its source column by id, and the ids are the
// schema's to give.
//
// day() and nothing else, on purpose. A landing table is read by day --
// yesterday's events, this month's -- and one transform every reader
// understands beats a grammar of bucket(16, x) and truncate(10, y) that
// the console would then have to display and this destination to check.
func partitionSpec(column string, sc *ice.Schema) (ice.PartitionSpec, error) {
	if column == "" {
		return *ice.UnpartitionedSpec, nil
	}
	f, ok := sc.FindFieldByName(column)
	if !ok {
		names := make([]string, 0, len(sc.Fields()))
		for _, f := range sc.Fields() {
			names = append(names, f.Name)
		}
		return ice.PartitionSpec{}, fmt.Errorf("iceberg.Table: PartitionBy %q is not a column of Schema; it takes one of %s",
			column, strings.Join(names, ", "))
	}
	if !f.Type.Equals(ice.PrimitiveTypes.TimestampTz) && !f.Type.Equals(ice.PrimitiveTypes.Date) {
		return ice.PartitionSpec{}, fmt.Errorf("iceberg.Table: PartitionBy %q is %s; day() partitions a timestamp or a date",
			column, f.Type)
	}
	// 1000 is the id the Iceberg spec reserves for the first partition
	// field, and `<column>_day` is the name every engine gives it.
	return ice.NewPartitionSpec(ice.PartitionField{
		SourceIDs: []int{f.ID}, FieldID: 1000, Name: column + "_day", Transform: ice.DayTransform{},
	}), nil
}

// checkPartition holds an existing table to the PartitionBy the consumer
// declared: the table must be partitioned by day of that column and nothing
// else. No PartitionBy is no claim, and the table's own spec stands. The
// table does not change either way -- a divergence is an error, not a
// migration (SDK_NEW_DRIVER.md, 2.9).
func checkPartition(spec ice.PartitionSpec, sc *ice.Schema, column string) error {
	if column == "" {
		return nil
	}
	if spec.NumFields() == 1 {
		f := spec.Field(0)
		if _, day := f.Transform.(ice.DayTransform); day && len(f.SourceIDs) == 1 {
			if src, ok := sc.FindFieldByID(f.SourceIDs[0]); ok && src.Name == column {
				return nil
			}
		}
	}
	return fmt.Errorf("iceberg.Table: PartitionBy %q, and the table is partitioned by %s; the table does not change, "+
		"so drop PartitionBy or set it to the table's column", column, describeSpec(spec, sc))
}

// describeSpec says how a table is partitioned the way Spark prints it:
// day(occurred_at), or "nothing".
func describeSpec(spec ice.PartitionSpec, sc *ice.Schema) string {
	if spec.IsUnpartitioned() {
		return "nothing"
	}
	parts := make([]string, 0, spec.NumFields())
	for i := 0; i < spec.NumFields(); i++ {
		f := spec.Field(i)
		cols := make([]string, 0, len(f.SourceIDs))
		for _, id := range f.SourceIDs {
			if src, ok := sc.FindFieldByID(id); ok {
				cols = append(cols, src.Name)
			} else {
				cols = append(cols, fmt.Sprintf("field %d", id))
			}
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", f.Transform, strings.Join(cols, ", ")))
	}
	return strings.Join(parts, ", ")
}
