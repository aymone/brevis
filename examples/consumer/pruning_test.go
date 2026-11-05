package consumer_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Dependency pruning is why the drivers live in subpackages, and it is
// measurable -- so it is asserted, not promised.
//
// Before phase 0 the root imported sdk/load, which imports BigQuery: 458 packages
// and 21 MB of binary for anyone who wanted to do Postgres -> Postgres. Go prunes
// by imported package, never by used field, so the only way not to pay for a
// driver is not to import its package.
func TestNotUsingBigQueryCompilesNoBigQuery(t *testing.T) {
	cases := []struct {
		name      string
		packages  []string
		forbidden bool
	}{
		{"the root alone", []string{"brevis.sh/brevis/sdk"}, true},
		{"root + from", []string{
			"brevis.sh/brevis/sdk",
			"brevis.sh/brevis/sdk/from",
		}, true},
		// A whole files pipeline -- from and to -- still does not pull BigQuery
		// in. This case was missing in v0.20.0, and without it the defect got
		// through: to.BigQuery and to.Files lived in the same package, so writing
		// a file compiled Google.
		{"root + from + to (files)", []string{
			"brevis.sh/brevis/sdk",
			"brevis.sh/brevis/sdk/from",
			"brevis.sh/brevis/sdk/to",
		}, true},

		// And the control: whoever asks for BigQuery gets BigQuery. Without it,
		// the test would pass with an SDK that loads nothing.
		{"root + to/bigquery", []string{
			"brevis.sh/brevis/sdk",
			"brevis.sh/brevis/sdk/to/bigquery",
		}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loads := strings.Contains(deps(t, c.packages...), "cloud.google.com/go/bigquery")

			if c.forbidden && loads {
				t.Error("BigQuery got into the graph of somebody who did not import it")
			}
			if !c.forbidden && !loads {
				t.Error("importing to.BigQuery has to bring BigQuery in; " +
					"without this the test above proves nothing")
			}
		})
	}
}

// The same reasoning for the object-storage backends. from.Files serves disk, S3
// and GCS, but the backend is a value -- so reading a local CSV compiles neither
// AWS nor Google. With the three in a single package, it would.
func TestReadingALocalFileCompilesNoCloud(t *testing.T) {
	cases := []struct {
		name     string
		packages []string
		search   string
		expected bool
	}{
		{"from on its own does not bring AWS in", []string{
			"brevis.sh/brevis/sdk/from"}, "aws-sdk-go", false},
		{"from on its own does not bring Google in", []string{
			"brevis.sh/brevis/sdk/from"}, "cloud.google.com", false},

		// The controls: whoever asks for a backend gets that backend.
		{"store/s3 brings in AWS", []string{
			"brevis.sh/brevis/sdk/store/s3"}, "aws-sdk-go", true},
		{"store/gcs brings in Google", []string{
			"brevis.sh/brevis/sdk/store/gcs"}, "cloud.google.com", true},
		{"store/s3 does not bring Google in", []string{
			"brevis.sh/brevis/sdk/store/s3"}, "cloud.google.com", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loads := strings.Contains(deps(t, c.packages...), c.search)
			if loads != c.expected {
				t.Errorf("carries %q = %v, expected %v", c.search, loads, c.expected)
			}
		})
	}
}

// deps runs in the sdk module, which is what declares those dependencies. Running
// it from here would only resolve what the examples module already imports.
func deps(t *testing.T, packages ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"list", "-deps"}, packages...)...)
	cmd.Dir = "../../sdk"
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	return string(out)
}
