package load

import (
	"strings"
	"testing"

	"brevis.sh/brevis/sdk/internal/core"
)

// TestI2CreationPlanNeverInfers is invariant I2 as a checkable
// property, with no BigQuery project at all.
//
// It was prose in a section called "where the discussion continues", and stayed
// open
// for months. What closes an invariant is not deciding it: it is being able to
// exercise it.
func TestI2CreationPlanNeverInfers(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *core.LoadConfig
		want    HowToCreate
		failure bool
	}{
		{
			"with a Schema, builds the DDL from the declaration",
			&core.LoadConfig{Schema: core.Schema{{Name: "a", Type: core.TypeString}}},
			CreateFromSchema, false,
		},
		{
			"with CreateSQL, runs the DDL its author wrote",
			&core.LoadConfig{CreateSQL: "CREATE TABLE t (a STRING)"},
			CreateFromSQL, false,
		},
		{
			"CreateSQL beats the Schema: a hand-written DDL says more",
			&core.LoadConfig{
				CreateSQL: "CREATE TABLE t (a NUMERIC(18,2))",
				Schema:    core.Schema{{Name: "a", Type: core.TypeNumeric}},
			},
			CreateFromSQL, false,
		},
		{
			"with neither, it REFUSES -- and that is what I2 demands",
			&core.LoadConfig{Columns: []string{"a", "b"}},
			0, true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CreationPlan(c.cfg, "d.t")
			if c.failure {
				if err == nil {
					t.Fatal("accepted creating a table without the types -- autodetect is back")
				}
				for _, want := range []string{"Target.Schema", "CreateSQL", "does not infer"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("the error does not say %q: %v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("CreationPlan: %v", err)
			}
			if got != c.want {
				t.Errorf("= %v, want %v", got, c.want)
			}
		})
	}
}

// TestI2SchemaBecomesBigQuerysType: the translation table, one row per type.
func TestI2SchemaBecomesBigQuerysType(t *testing.T) {
	s := core.Schema{
		{Name: "text", Type: core.TypeString, Required: true},
		{Name: "integer", Type: core.TypeInt64},
		{Name: "float", Type: core.TypeFloat64},
		{Name: "decimal", Type: core.TypeNumeric},
		{Name: "boolean", Type: core.TypeBool},
		{Name: "instant", Type: core.TypeTimestamp},
		{Name: "data", Type: core.TypeDate},
		{Name: "document", Type: core.TypeJSON},
		{Name: "bytes", Type: core.TypeBytes},
	}

	got, err := bigquerySchema(s)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"text": "STRING", "integer": "INTEGER", "float": "FLOAT",
		"decimal": "NUMERIC", "boolean": "BOOLEAN", "instant": "TIMESTAMP",
		"data": "DATE", "document": "JSON", "bytes": "BYTES",
	}
	if len(got) != len(expected) {
		t.Fatalf("%d columns, want %d", len(got), len(expected))
	}
	for _, f := range got {
		if string(f.Type) != expected[f.Name] {
			t.Errorf("%s = %s, want %s", f.Name, f.Type, expected[f.Name])
		}
	}
	if !got[0].Required {
		t.Error("Required did not become NOT NULL")
	}
}

// TestI2TheSDKsColumnsBeatTheDeclaration: ingestion_id and ingestion_loaded_at
// are
// the SDK's, and a NULLABLE there would let the dedup match on null.
func TestI2TheSDKsColumnsBeatTheDeclaration(t *testing.T) {
	got, err := bigquerySchema(core.Schema{
		{Name: core.MetadataID, Type: core.TypeInt64},        // wrong on purpose
		{Name: core.MetadataLoadedAt, Type: core.TypeString}, // wrong on purpose
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got[0].Type) != "STRING" || !got[0].Required {
		t.Errorf("%s = %s required=%v; the SDK's shape should win",
			core.MetadataID, got[0].Type, got[0].Required)
	}
	if string(got[1].Type) != "TIMESTAMP" || !got[1].Required {
		t.Errorf("%s = %s required=%v", core.MetadataLoadedAt, got[1].Type, got[1].Required)
	}
}

// TestI4ThePartitionComesFromTheDeclaration.
func TestI4ThePartitionComesFromTheDeclaration(t *testing.T) {
	if got := partitionOf(&core.LoadConfig{PartitionBy: "my_column"}); got != "my_column" {
		t.Errorf("= %q, the declaration should win", got)
	}
	if got := partitionOf(&core.LoadConfig{}); got != core.MetadataLoadedAt {
		t.Errorf("= %q, with no declaration the documented default stays", got)
	}
}
