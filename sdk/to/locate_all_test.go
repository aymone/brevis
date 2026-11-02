package to_test

import (
	"testing"

	"brevis.sh/brevis/sdk"
	"brevis.sh/brevis/sdk/to"
	"brevis.sh/brevis/sdk/to/bigquery"
	"brevis.sh/brevis/sdk/to/mysql"
	"brevis.sh/brevis/sdk/to/postgres"
	"brevis.sh/brevis/sdk/to/pubsub"
	"brevis.sh/brevis/sdk/to/redshift"
)

// Every writer this SDK ships can name its destination. A writer added without
// Locate would load correctly and never appear on /data, and nothing else
// would notice: add it to this list.
func TestEveryShippedWriterIsALocator(t *testing.T) {
	writers := map[string]sdk.Writer{
		"to.Files":       to.Files{},
		"bigquery.Table": bigquery.Table{},
		"postgres.Table": postgres.Table{},
		"mysql.Table":    mysql.Table{},
		"redshift.Table": redshift.Table{},
		"pubsub.Topic":   pubsub.Topic{},
	}
	for name, w := range writers {
		if _, ok := w.(sdk.Locator); !ok {
			t.Errorf("%s does not implement sdk.Locator", name)
		}
	}
}
