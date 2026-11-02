package iceberg

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk"
)

// THE MATRIX ROW, AS A TEST. sdk/capabilities_test.go holds every
// destination's row and checks it against the code, so that no driver
// "accepts and ignores" an option; it cannot import this module, which
// depends on the sdk, so the row lives here and SDK_MATRIX.md copies it:
//
//	iceberg.Table | Dedup refused, naming Dedup and BatchKey | CreateTable yes, from Schema | Preview as to.Files
func TestTheMatrixRow(t *testing.T) {
	tbl := Table{Catalog: Catalog{Name: "lake", Type: SQL, DSN: "postgres://x:secret@y/z"}, Namespace: "bronze", Name: "events"}

	_, err := tbl.Write(context.Background(),
		[]sdk.Envelope{{Payload: map[string]any{"ingestion_id": "x"}}},
		sdk.WriteOptions{Dedup: sdk.DedupMerge})
	if err == nil {
		t.Fatal("Dedup accepted; a written flag that does nothing is worse than an error")
	}
	if !strings.Contains(err.Error(), "Dedup") {
		t.Errorf("the refusal does not name Dedup: %v", err)
	}

	f, ok := reflect.TypeOf(tbl).FieldByName("CreateTable")
	if !ok || f.Type != reflect.TypeOf((*bool)(nil)) {
		t.Errorf("CreateTable: the matrix says this destination creates a table from Schema, tri-state like BigQuery")
	}

	// Evolve is a field of the destinations that evolve -- postgres.Table,
	// bigquery.Table -- and not of WriteOptions. This destination does not
	// have one: a divergence between the batch and the table is Reconcile's
	// error, never an ALTER, and a field that evolved nothing would be the
	// dead flag the matrix exists to prevent.
	if _, has := reflect.TypeOf(tbl).FieldByName("Evolve"); has {
		t.Error("Evolve: this destination does not evolve a schema, so the field must not exist")
	}

	for _, secret := range []string{"secret", "://x", "password"} {
		if strings.Contains(tbl.Describe(), secret) {
			t.Errorf("Describe() = %q carries %q", tbl.Describe(), secret)
		}
	}
}
