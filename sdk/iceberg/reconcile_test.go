package iceberg

import (
	"strconv"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk"
	ice "github.com/apache/iceberg-go"
)

// landing is the table most of these tests reconcile against: a landing
// table created from a declaration, as the catalog gives it back.
var landing = sdk.Schema{
	{Name: "event_id", Type: sdk.TypeString, Required: true},
	{Name: "amount", Type: sdk.TypeNumeric},
	{Name: "note", Type: sdk.TypeString},
	{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
}

const loadedAt = "2026-11-02T12:00:00Z"

func tableOf(t *testing.T, s sdk.Schema) *ice.Schema {
	t.Helper()
	is, err := IcebergSchema(s)
	if err != nil {
		t.Fatal(err)
	}
	return is
}

type fields = map[string]any

func rows(fs ...fields) []sdk.Envelope {
	out := make([]sdk.Envelope, len(fs))
	for i, f := range fs {
		out[i] = row(strconv.Itoa(i), f)
	}
	return out
}

func names(s sdk.Schema) string { return strings.Join(s.Names(), " ") }

// A FIELD THE TABLE DOES NOT HAVE IS REFUSED NAMING THE FIELD (rule 2.8, first
// line). Carrying on would drop that data with no signal at all, which is the
// worst way to fail. A field that is nil in every record and that the table
// does not have is not that: nothing would have been written, so nothing is
// refused -- a null and an absent field land the same NULL.
func TestAFieldTheTableDoesNotHaveIsRefusedByName(t *testing.T) {
	tbl := tableOf(t, landing)
	_, err := Reconcile(tbl, rows(
		fields{"event_id": "a", "amount": "1", "ingestion_loaded_at": loadedAt, "extra": 1},
	), sdk.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "extra") || !strings.Contains(err.Error(), "does not have") {
		t.Fatalf("a field the table lacks: %v, want a refusal naming it", err)
	}

	_, err = Reconcile(tbl, rows(
		fields{"event_id": "a", "ingestion_loaded_at": loadedAt, "ghost": nil},
		fields{"event_id": "b", "ingestion_loaded_at": loadedAt},
	), sdk.WriteOptions{})
	if err != nil {
		t.Fatalf("a field nil in every record and absent from the table: %v, want nothing refused", err)
	}
}

// A TABLE COLUMN THE RECORDS DO NOT CARRY STAYS NULL (second line): a landing
// table legitimately has a column the source sometimes does not send. The
// schema that comes back still has every table column, in the table's order,
// so Records writes the null there instead of a batch missing a column.
func TestATableColumnTheRecordsDoNotCarryStaysNull(t *testing.T) {
	tbl := tableOf(t, landing)
	in := rows(fields{"event_id": "a", "ingestion_loaded_at": loadedAt})
	s, err := Reconcile(tbl, in, sdk.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(s), names(landing); got != want {
		t.Fatalf("schema %q, want the table's %q", got, want)
	}
	rec := one(t, s, in)
	if !rec.Column(1).IsNull(0) || !rec.Column(2).IsNull(0) {
		t.Errorf("amount and note should be null; got %v %v", rec.Column(1).IsNull(0), rec.Column(2).IsNull(0))
	}
	if rec.Column(0).IsNull(0) || rec.Column(3).IsNull(0) {
		t.Errorf("event_id and ingestion_loaded_at carried values")
	}
}

// INCOMPATIBLE TYPES UNDER ONE NAME ARE REFUSED NAMING THE COLUMN AND BOTH
// TYPES (third line). Two shapes of it: the consumer declared a type the
// table does not hold, and the table holds a type this destination does not
// write -- a decimal(18, 2) created by someone else, say. The second is a
// refusal too, because writing decimal(38, 9) values into it would be a
// guess about rounding nobody asked for.
func TestIncompatibleTypesAreRefusedNamingBoth(t *testing.T) {
	declared := sdk.Schema{
		{Name: "event_id", Type: sdk.TypeString, Required: true},
		{Name: "amount", Type: sdk.TypeFloat64},
		{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
	}
	_, err := Reconcile(tableOf(t, landing),
		rows(fields{"event_id": "a", "amount": 1.5, "ingestion_loaded_at": loadedAt}),
		sdk.WriteOptions{Columns: declared.Names(), Schema: declared})
	for _, want := range []string{`"amount"`, "decimal(38, 9)", "float64"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("declared float64 against a decimal column: %v, want a refusal naming %s", err, want)
		}
	}

	foreign := ice.NewSchema(0,
		ice.NestedField{ID: 1, Name: "event_id", Type: ice.PrimitiveTypes.String, Required: true},
		ice.NestedField{ID: 2, Name: "amount", Type: ice.DecimalTypeOf(18, 2)},
	)
	_, err = Reconcile(foreign, rows(fields{"event_id": "a"}), sdk.WriteOptions{})
	for _, want := range []string{`"amount"`, "decimal(18, 2)", "decimal(38, 9)"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a table type this destination does not write: %v, want a refusal naming %s", err, want)
		}
	}
}

// A REQUIRED TABLE COLUMN NO RECORD CARRIES IS REFUSED (fourth line), before
// a Parquet file with an all-null required column is written. One record
// carrying it is enough HERE: the record that does not is Records' refusal,
// with the record's name in it.
func TestARequiredTableColumnNoRecordCarriesIsRefused(t *testing.T) {
	tbl := tableOf(t, landing)
	_, err := Reconcile(tbl, rows(
		fields{"amount": "1", "ingestion_loaded_at": loadedAt},
		fields{"amount": "2", "ingestion_loaded_at": loadedAt},
	), sdk.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), `"event_id"`) || !strings.Contains(err.Error(), "required") {
		t.Fatalf("no record carries event_id: %v, want a refusal naming it as required", err)
	}
	if _, err := Reconcile(tbl, rows(
		fields{"event_id": "a", "ingestion_loaded_at": loadedAt},
		fields{"ingestion_loaded_at": loadedAt},
	), sdk.WriteOptions{}); err != nil {
		t.Fatalf("one record carries event_id: %v, want the batch accepted here", err)
	}
}

// A DECLARED COLUMN THE CHAIN DOES NOT PRODUCE IS ITS OWN REFUSAL, distinct
// from a table column nobody sent. "You declared `note` and your Transform
// does not produce it" is a bug worth stopping for; "the table has `note`
// and this source does not send it" is what a landing table is. Same absent
// column, two different situations, two different messages -- and the
// other half of the declaration holds too: a field with a value that Columns
// does not list stops the load.
func TestADeclaredColumnTheChainDoesNotProduceIsItsOwnRefusal(t *testing.T) {
	tbl := tableOf(t, landing)
	in := rows(fields{"event_id": "a", "amount": "1", "ingestion_loaded_at": loadedAt})

	_, err := Reconcile(tbl, in, sdk.WriteOptions{Columns: []string{"event_id", "amount", "note", "ingestion_loaded_at"}})
	if err == nil || !strings.Contains(err.Error(), "note") || !strings.Contains(err.Error(), "Columns") {
		t.Fatalf("declared note, not produced: %v, want a refusal about the Columns declaration", err)
	}
	if strings.Contains(err.Error(), "the table does not have") {
		t.Errorf("the declaration's refusal reads like the table's: %v", err)
	}

	if _, err := Reconcile(tbl, in, sdk.WriteOptions{}); err != nil {
		t.Fatalf("note in the table, nobody declared or sent it: %v, want null and no refusal", err)
	}

	_, err = Reconcile(tbl, in, sdk.WriteOptions{Columns: []string{"event_id", "ingestion_loaded_at"}})
	if err == nil || !strings.Contains(err.Error(), "amount") || !strings.Contains(err.Error(), "Columns does not declare") {
		t.Fatalf("amount carried, not declared: %v, want a refusal naming it", err)
	}
}

// THE SCHEMA COMES BACK IN THE TABLE'S ORDER, WITH THE DECLARED TYPE WHERE
// THERE IS ONE: a table `string` is text until the consumer says it is a
// document, and `json` is how they say it -- the same Iceberg type, a
// different conversion. Required is the table's, or the declaration's when
// it is stricter. And it is a pure function: a table schema in, a schema
// out, no catalog, no context.
func TestTheSchemaComesBackInTableOrderWithDeclaredTypesRefiningIt(t *testing.T) {
	tbl := tableOf(t, sdk.Schema{
		{Name: "event_id", Type: sdk.TypeString, Required: true},
		{Name: "attrs", Type: sdk.TypeString},
		{Name: "amount", Type: sdk.TypeNumeric},
	})
	declared := sdk.Schema{ // another order, and attrs as a document
		{Name: "amount", Type: sdk.TypeNumeric, Required: true},
		{Name: "attrs", Type: sdk.TypeJSON},
		{Name: "event_id", Type: sdk.TypeString},
	}
	in := rows(fields{"event_id": "a", "attrs": map[string]any{"k": 1}, "amount": "1"})
	s, err := Reconcile(tbl, in, sdk.WriteOptions{Columns: declared.Names(), Schema: declared})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(s); got != "event_id attrs amount" {
		t.Errorf("order %q, want the table's", got)
	}
	want := sdk.Schema{
		{Name: "event_id", Type: sdk.TypeString, Required: true}, // the table's
		{Name: "attrs", Type: sdk.TypeJSON},                      // the declaration's type
		{Name: "amount", Type: sdk.TypeNumeric, Required: true},  // the declaration's, stricter
	}
	for i := range want {
		if s[i] != want[i] {
			t.Errorf("column %d: %+v, want %+v", i, s[i], want[i])
		}
	}
	rec := one(t, s, in)
	if got := value(rec.Column(1), 0); got != `{"k":1}` {
		t.Errorf("attrs converted as %v, want the document", got)
	}
}
