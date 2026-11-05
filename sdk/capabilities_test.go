package sdk_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk"
	"brevis.sh/brevis/sdk/to"
	"brevis.sh/brevis/sdk/to/bigquery"
	"brevis.sh/brevis/sdk/to/mysql"
	"brevis.sh/brevis/sdk/to/postgres"
	"brevis.sh/brevis/sdk/to/pubsub"
	"brevis.sh/brevis/sdk/to/redshift"
)

// The compatibility matrix, as a TEST and not as a table in a .md file.
//
// §5 of the drivers' plan names the risk: nine drivers with Metadata, Dedup,
// CreateTable and Preview are 36 combinations, and promising all 36 without
// measuring is how `DeleteAfterLoad` reached the documentation with a default it
// did not have.
//
// What this file prevents is the third answer. For each combination there are
// only two acceptable ones:
//
//	supported   the driver does it
//	refused     the driver says it does not, naming the field
//
// The one that must not exist is "accepts and ignores": a written flag that does
// nothing is the class of defect this project has found most often in itself.

// support declares what each destination does with each option. It is this table
// the documentation copies -- and it is the one the test below checks against the
// code.
type support struct {
	dedup       bool
	createTable bool
}

var destinations = map[string]struct {
	writer  sdk.Writer
	support support
	// refusalHint is what the message has to contain when the driver refuses,
	// so whoever reads it knows what to do.
	refusalHint string
}{
	"to.Files": {
		writer:      to.Files{Path: "/tmp/brevis-cap"},
		support:     support{dedup: false, createTable: false},
		refusalHint: "Dedup",
	},
	"postgres.Table": {
		writer:  postgres.Table{DSN: "postgres://x/y", Name: "t"},
		support: support{dedup: true, createTable: true},
	},
	"mysql.Table": {
		writer:  mysql.Table{DSN: "u@tcp(x)/y", Name: "t"},
		support: support{dedup: true, createTable: true},
	},
	"redshift.Table": {
		writer:  redshift.Table{DSN: "postgres://x/y", Name: "t"},
		support: support{dedup: true, createTable: false},
	},
	"bigquery.Table": {
		writer:  bigquery.Table{Project: "p", Dataset: "d", Name: "t"},
		support: support{dedup: true, createTable: true},
	},
	// The first destination that is neither a table nor a directory, and it
	// refuses MORE than any other here: a Schema (which exists so a destination
	// can create its table, and a topic exists already) and a PartitionBy on
	// top of Dedup.
	"pubsub.Topic": {
		writer:      pubsub.Topic{Project: "p", Name: "t"},
		support:     support{dedup: false, createTable: false},
		refusalHint: "at-least-once",
	},
}

// TestDedupEitherWorksOrRefuses: no destination may accept Dedup and not
// deduplicate.
func TestDedupEitherWorksOrRefuses(t *testing.T) {
	for name, d := range destinations {
		t.Run(name, func(t *testing.T) {
			_, err := d.writer.Write(context.Background(),
				[]sdk.Envelope{{Payload: map[string]any{"ingestion_id": "x"}}},
				sdk.WriteOptions{Dedup: sdk.DedupMerge})

			if !d.support.dedup {
				if err == nil {
					t.Fatalf("%s accepted Dedup without supporting it -- a flag that is written "+
						"and does nothing is worse than an error", name)
				}
				if d.refusalHint != "" && !strings.Contains(err.Error(), d.refusalHint) {
					t.Errorf("the refusal does not name %q: %v", d.refusalHint, err)
				}
				return
			}

			// Supported: whatever error comes has to be a connection one, and
			// not a refusal of the option. A driver that supports Dedup but
			// rejects it in validation would be the same lie in reverse.
			if err != nil && strings.Contains(err.Error(), "does not have") &&
				strings.Contains(err.Error(), "Dedup") {
				t.Errorf("%s claims to support Dedup and refused it: %v", name, err)
			}
		})
	}
}

// TestNoSQLDestinationInventsAType: guessing NUMERIC(18,2) from a JSON number
// is still the one thing this SDK will not do.
//
// What changed is where the shape comes FROM. Postgres and MySQL now create a
// table -- from a declared Schema or from CreateSQL, never from the batch -- so
// they carry a CreateTable field and the matrix above says so. Redshift and
// to.Files do not, and for them this pins the absence: a CreateTable field that
// created nothing would be exactly the dead flag.
//
// This test is why the matrix had to move rather than drift. It failed the
// moment the two drivers grew the field, which is what a capability matrix is
// for.
func TestNoSQLDestinationInventsAType(t *testing.T) {
	for name, d := range destinations {
		if d.support.createTable {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if hasField(d.writer, "CreateTable") {
				t.Errorf("%s has a CreateTable field and the matrix says it does not create tables; "+
					"either the field does something, or it should not exist", name)
			}
		})
	}
}

// TestEveryDestinationDescribesItselfWithoutASecret: Describe reaches the log,
// the Result and error messages, and DSNs carry passwords.
func TestEveryDestinationDescribesItselfWithoutASecret(t *testing.T) {
	secrets := []string{"senha", "secret", "password", "@tcp", "://x/y"}
	for name, d := range destinations {
		t.Run(name, func(t *testing.T) {
			desc := d.writer.Describe()
			if desc == "" {
				t.Fatal("empty Describe")
			}
			for _, s := range secrets {
				if strings.Contains(desc, s) {
					t.Errorf("Describe() = %q, and it carries %q", desc, s)
				}
			}
		})
	}
}

// hasField says whether the driver's type declares the field.
func hasField(w sdk.Writer, name string) bool {
	t := reflect.TypeOf(w)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, tem := t.FieldByName(name)
	return tem
}
