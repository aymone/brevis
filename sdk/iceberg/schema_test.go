package iceberg

import (
	"strings"
	"testing"

	"brevis.sh/brevis/sdk"
	"github.com/apache/arrow-go/v18/arrow"
)

// ONE SDK TYPE, ONE ICEBERG TYPE, ONE ARROW TYPE, written down rather than
// inferred. This table is the spec's, and it is what the data files will
// carry for as long as the destination exists: a reader in two years meets
// the type chosen here, so the choice is a test and not a comment.
func TestEachSDKTypeHasOneIcebergTypeAndOneArrowType(t *testing.T) {
	cases := []struct {
		sdk     sdk.ColumnType
		iceberg string
		arrow   arrow.DataType
	}{
		{sdk.TypeString, "string", arrow.BinaryTypes.String},
		{sdk.TypeInt64, "long", arrow.PrimitiveTypes.Int64},
		{sdk.TypeFloat64, "double", arrow.PrimitiveTypes.Float64},
		// decimal(38, 9): BigQuery's NUMERIC, which is what the SDK's
		// `numeric` has always meant, so a value lands the same in a lake
		// and in a warehouse.
		{sdk.TypeNumeric, "decimal(38, 9)", &arrow.Decimal128Type{Precision: 38, Scale: 9}},
		{sdk.TypeBool, "boolean", arrow.FixedWidthTypes.Boolean},
		// timestamptz, microseconds: an SDK `timestamp` is an instant with a
		// zone, and Iceberg v2 keeps microseconds.
		{sdk.TypeTimestamp, "timestamptz", &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"}},
		{sdk.TypeDate, "date", arrow.FixedWidthTypes.Date32},
		// json is a string: the serialized document. Iceberg v3 has a
		// variant type, and DuckDB does not read it yet; a string every
		// reader parses beats a type half of them refuse.
		{sdk.TypeJSON, "string", arrow.BinaryTypes.String},
		{sdk.TypeBytes, "binary", arrow.BinaryTypes.Binary},
	}
	for _, c := range cases {
		t.Run(string(c.sdk), func(t *testing.T) {
			s := sdk.Schema{{Name: "c", Type: c.sdk}}
			is, err := IcebergSchema(s)
			if err != nil {
				t.Fatalf("IcebergSchema: %v", err)
			}
			if got := is.Fields()[0].Type.String(); got != c.iceberg {
				t.Errorf("iceberg type %q, want %q", got, c.iceberg)
			}
			as, err := ArrowSchema(s)
			if err != nil {
				t.Fatalf("ArrowSchema: %v", err)
			}
			if got := as.Field(0).Type; !arrow.TypeEqual(got, c.arrow) {
				t.Errorf("arrow type %s, want %s", got, c.arrow)
			}
		})
	}
}

// REQUIRED IS REQUIRED ON BOTH SIDES, and field ids follow the declaration
// from 1. A partition spec names its source column by id and a reader
// matches Parquet columns to the table by id, so the numbering is part of
// the contract and not a detail of construction.
func TestRequiredAndFieldIDsFollowTheDeclaration(t *testing.T) {
	s := sdk.Schema{
		{Name: "event_id", Type: sdk.TypeString, Required: true},
		{Name: "amount", Type: sdk.TypeNumeric},
		{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
	}
	is, err := IcebergSchema(s)
	if err != nil {
		t.Fatal(err)
	}
	as, err := ArrowSchema(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(is.Fields()) != len(s) || as.NumFields() != len(s) {
		t.Fatalf("fields: iceberg %d, arrow %d, want %d", len(is.Fields()), as.NumFields(), len(s))
	}
	for i, c := range s {
		f := is.Fields()[i]
		if f.ID != i+1 {
			t.Errorf("%s: iceberg field id %d, want %d", c.Name, f.ID, i+1)
		}
		if f.Name != c.Name || as.Field(i).Name != c.Name {
			t.Errorf("column %d: names %q / %q, want %q", i, f.Name, as.Field(i).Name, c.Name)
		}
		if f.Required != c.Required {
			t.Errorf("%s: iceberg required %v, declared %v", c.Name, f.Required, c.Required)
		}
		if as.Field(i).Nullable == c.Required {
			t.Errorf("%s: arrow nullable %v, declared required %v", c.Name, as.Field(i).Nullable, c.Required)
		}
	}
}

// A TYPE THIS TABLE DOES NOT KNOW IS REFUSED BY NAME, the column and the
// type both, because the alternative -- a guess -- is the defect this SDK
// refuses everywhere (SDK_NEW_DRIVER.md, rule 2.5). And a schema with no
// columns is refused before it becomes a table with none.
func TestATypeOrASchemaThisTableCannotHoldIsRefused(t *testing.T) {
	bad := sdk.Schema{{Name: "id", Type: sdk.ColumnType("uuid")}}
	for name, f := range map[string]func(sdk.Schema) error{
		"IcebergSchema": func(s sdk.Schema) error { _, err := IcebergSchema(s); return err },
		"ArrowSchema":   func(s sdk.Schema) error { _, err := ArrowSchema(s); return err },
	} {
		err := f(bad)
		if err == nil || !strings.Contains(err.Error(), `"id"`) || !strings.Contains(err.Error(), `"uuid"`) {
			t.Errorf("%s(uuid column): %v, want an error naming the column and the type", name, err)
		}
		if err := f(sdk.Schema{}); err == nil {
			t.Errorf("%s(no columns): accepted", name)
		}
	}
}
