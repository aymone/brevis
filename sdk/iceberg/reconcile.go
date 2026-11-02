package iceberg

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AreteAcademy/brevis/sdk"
	ice "github.com/apache/iceberg-go"
)

// Reconcile decides, before a byte is written, what the batch becomes in the
// table: the schema Records converts with, every table column in the table's
// order, with the type the conversion will use and whether null is allowed.
//
// The table is the authority, and the rule is asymmetric (SDK_NEW_DRIVER.md,
// 2.8), because the two directions fail differently:
//
//   - a field the records carry that the table does not have is an ERROR
//     naming the field. Carrying on would drop that data with no signal at
//     all, which is the worst way to fail.
//   - a table column the records do not carry stays NULL. A landing table
//     legitimately does that, so the column is in the schema and Records
//     writes the null.
//   - incompatible types under one name are an ERROR naming the column and
//     both types -- the consumer's declaration against the table's, and the
//     table's against what this destination is able to write.
//   - a required column no record carries is an ERROR naming it, before a
//     Parquet file with an all-null required column exists.
//
// Before the table, the DECLARATION: a Columns entry the Transform chain did
// not produce is a refusal of its own, distinct from a table column nobody
// sent. The first is the consumer's bug; the second is a landing table.
//
// Pure: a table schema in, a schema out. It opens nothing and takes no
// context, so the whole rule is testable without a catalog.
func Reconcile(table *ice.Schema, batch []sdk.Envelope, opt sdk.WriteOptions) (sdk.Schema, error) {
	if err := checkDeclaration(opt, batch); err != nil {
		return nil, err
	}
	valued := valuedFields(batch)

	var extra []string
	for name := range valued {
		if _, ok := table.FindFieldByName(name); !ok {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf("iceberg: the rows carry column(s) %s, which the table does not have. "+
			"They would be silently dropped, so the load stops here: add the column to the table, "+
			"or remove the field in Transform", strings.Join(extra, ", "))
	}

	declared := make(map[string]sdk.Column, len(opt.Schema))
	for _, c := range opt.Schema {
		declared[c.Name] = c
	}
	out := make(sdk.Schema, 0, table.NumFields())
	var required []string
	for _, f := range table.Fields() {
		c, err := columnFor(f, declared)
		if err != nil {
			return nil, err
		}
		if c.Required && len(batch) > 0 && !valued[f.Name] {
			required = append(required, f.Name)
		}
		out = append(out, c)
	}
	if len(required) > 0 {
		return nil, fmt.Errorf("iceberg: column(s) %s are required in the table and no record of the batch "+
			"carries a value for them", quoted(required))
	}
	return out, nil
}

// columnFor is one table column as the conversion will see it: the declared
// type when the consumer declared one and it fits the table, otherwise the
// table's type read back through the type map. Required is the table's, or
// the declaration's when that is stricter.
func columnFor(f ice.NestedField, declared map[string]sdk.Column) (sdk.Column, error) {
	if d, ok := declared[f.Name]; ok {
		m, known := types[d.Type]
		if !known {
			return sdk.Column{}, fmt.Errorf("iceberg: column %q has type %q, which this destination has no Iceberg type for; it takes %s",
				d.Name, d.Type, knownTypes())
		}
		if !m.iceberg.Equals(f.Type) {
			return sdk.Column{}, fmt.Errorf("iceberg: column %q is %s in the table and declared %s, which this destination writes as %s; "+
				"the table does not change, so change the declaration or the table",
				f.Name, f.Type, d.Type, m.iceberg)
		}
		return sdk.Column{Name: f.Name, Type: d.Type, Required: f.Required || d.Required}, nil
	}
	t, ok := sdkTypeOf(f.Type)
	if !ok {
		return sdk.Column{}, fmt.Errorf("iceberg: column %q is %s in the table, a type this destination does not write; "+
			"it writes %s. Declare the column with a type from that list, against a table that holds it",
			f.Name, f.Type, writtenTypes())
	}
	return sdk.Column{Name: f.Name, Type: t, Required: f.Required}, nil
}

// readBack is the type map walked the other way, in a fixed order, for a
// table column nobody declared. `json` is not in it: a table `string` is
// text until the consumer says it is a document, and a declaration is how
// they say it.
var readBack = []sdk.ColumnType{
	sdk.TypeString, sdk.TypeInt64, sdk.TypeFloat64, sdk.TypeNumeric,
	sdk.TypeBool, sdk.TypeTimestamp, sdk.TypeDate, sdk.TypeBytes,
}

func sdkTypeOf(t ice.Type) (sdk.ColumnType, bool) {
	for _, s := range readBack {
		if types[s].iceberg.Equals(t) {
			return s, true
		}
	}
	return "", false
}

// writtenTypes lists the Iceberg types this destination writes, for a
// refusal of a table that holds something else.
func writtenTypes() string {
	names := make([]string, 0, len(readBack))
	for _, s := range readBack {
		names = append(names, types[s].iceberg.String())
	}
	return strings.Join(names, ", ")
}

// checkDeclaration holds the consumer to their Columns, in both directions,
// the way every destination in the SDK does: a declared column the chain did
// not deliver is an error naming it, and a field with a value that the
// declaration does not list is an error naming it. Checked on the first
// record, because every record of a batch comes out of one Transform chain.
//
// Names are exact. Iceberg field names are case-sensitive, and folding here
// could only let more through on a guess.
func checkDeclaration(opt sdk.WriteOptions, batch []sdk.Envelope) error {
	if len(opt.Columns) == 0 || len(batch) == 0 {
		return nil
	}
	row, err := object(batch[0].Payload)
	if err != nil {
		return fmt.Errorf("iceberg: the Columns declaration can only be checked against a JSON object: %w", err)
	}
	want := make(map[string]bool, len(opt.Columns))
	var missing []string
	for _, c := range opt.Columns {
		want[c] = true
		if _, present := row[c]; !present {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("iceberg: the Columns declaration lists %s, which the row does not have. "+
			"Compose them in Transform -- sdk.IngestionID() and sdk.IngestionLoadedAt() write the two the SDK knows. "+
			"The row has: %s", strings.Join(missing, ", "), keysOf(row))
	}
	// A field nil in every record is not an undeclared column: nothing
	// declared it because it has no shape, and nothing would be written.
	valued := valuedFields(batch)
	var undeclared []string
	for f := range row {
		if !want[f] && valued[f] {
			undeclared = append(undeclared, f)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf("iceberg: the row carries %s, which Columns does not declare. "+
			"They would be written to a table that never mentioned them, so the load stops here: "+
			"add them to Columns, or drop them in Transform", strings.Join(undeclared, ", "))
	}
	return nil
}

// valuedFields names the fields some record of the batch gave a value. PER
// BATCH, not per record: a field with a value in record 5 is a column the
// table needs, and the records that sent nil for it write NULL there. A
// record that is not an object is skipped here; Records refuses it with the
// record's name.
func valuedFields(batch []sdk.Envelope) map[string]bool {
	valued := map[string]bool{}
	for _, e := range batch {
		row, err := object(e.Payload)
		if err != nil {
			continue
		}
		for k, v := range row {
			if v != nil {
				valued[k] = true
			}
		}
	}
	return valued
}

func keysOf(row map[string]any) string {
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func quoted(names []string) string {
	sort.Strings(names)
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}
