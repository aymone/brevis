package pages

import (
	"testing"
)

// THE CONNECTIONS THIS CONSOLE CANNOT READ ARE NEVER THE ONE IT OPENS ON.
func TestTheWorkbenchOpensOnAConnectionItReads(t *testing.T) {
	if !Queryable("bigquery://acme/bronze/orders") || !Queryable("postgres://shop/public/invoices") ||
		Queryable("redshift://cluster/sales/events") || Queryable("mysql://shop/orders") {
		t.Error("Queryable")
	}
	v := SQLView{Targets: []string{"redshift://cluster/sales/events", "postgres://shop/public/invoices"}}
	if got := v.FirstQueryable(); got != "postgres://shop/public/invoices" {
		t.Errorf("opens on %q", got)
	}
}
