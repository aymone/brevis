package load

import (
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"
)

// Criterion 2: a declared column neither Transform nor Metadata delivers.
func TestColumnsRefusesAColumnTheTableLacks(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "ingestion_id", Type: bigquery.StringFieldType},
		{Name: "payload", Type: bigquery.JSONFieldType},
	}

	err := checkDeclaredAgainstTable(
		[]string{"ingestion_id", "payload", "provider"}, schema, "bronze.orders")
	if err == nil {
		t.Fatal("a declared column that the table does not have is a load that does not work")
	}
	for _, want := range []string{"provider", "bronze.orders", "ingestion_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q (it must name both sides): %v", want, err)
		}
	}
}

// Asymmetric, like reconcile: a table column the declaration omits stays
// NULL, and a landing table legitimately has those.
func TestColumnsAcceptsAnUndeclaredTableColumn(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "ingestion_id", Type: bigquery.StringFieldType},
		{Name: "payload", Type: bigquery.JSONFieldType},
		{Name: "source_key", Type: bigquery.StringFieldType},
	}
	if err := checkDeclaredAgainstTable([]string{"ingestion_id", "payload"}, schema, "t"); err != nil {
		t.Errorf("a table column that the declaration does not list stays NULL, and that is legitimate: %v", err)
	}
}
