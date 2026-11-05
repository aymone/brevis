package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	domain "brevis.sh/brevis/internal/domain/workflow"
)

// roundTrip publishes the YAML a workflow is shown as, and says whether it
// is the same workflow.
func roundTrip(t *testing.T, name string, w domain.Workflow) {
	t.Helper()
	y, err := YAML(w)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	back, err := Parse(name, y)
	if err != nil {
		t.Fatalf("%s: the YAML it is shown as does not parse: %v\n%s", name, err, y)
	}
	if !reflect.DeepEqual(back, w) {
		t.Errorf("%s: the YAML it is shown as is another workflow\n got: %+v\nwant: %+v\n%s", name, back, w, y)
	}
}

// THE DEFINITION TAB SHOWS A YAML THAT PUBLISHES THE SAME WORKFLOW: every
// workflow in the repository, written back as YAML and read again, is the
// workflow it was. A YAML that published something else would be a lie with
// a Copy button.
func TestEveryExampleRoundTripsThroughItsYAML(t *testing.T) {
	n := 0
	for _, root := range []string{"../../../examples", "../../../examples/quickstart/workflows"} {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			w, err := Parse(e.Name(), raw)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip(t, e.Name(), w)
			n++
		}
	}
	if n < 5 {
		t.Fatalf("only %d examples were read", n)
	}
}

// EVERYTHING A WORKFLOW CAN SAY survives the trip: a chain, labeled
// dependencies, a concurrency group, a debounce, the four presentation keys
// of a param, a step's every field.
func TestAWorkflowWithEverythingRoundTrips(t *testing.T) {
	yes := true
	dag := domain.Workflow{
		Slug: "everything", Name: "everything", Kind: domain.KindDAG, Schedule: "0 4 * * *",
		Description: "All of it.", Tags: []string{"a", "b"}, Image: "python:3.12",
		Resources: domain.Resources{CPU: "500m", Memory: "1Gi", MemoryLimit: "2Gi"},
		Env:       map[string]string{"LEVEL": "info"}, Secrets: map[string]string{"TOKEN": "warehouse/token"},
		MaxActive: 1, Group: "warehouse",
		Trigger: domain.Trigger{OnLanded: []string{"bigquery://acme/bronze/orders"}, Debounce: 5 * time.Minute},
		Params: []domain.Param{
			{Name: "full", Type: domain.ParamBool, Default: "false", Title: "Full load", Group: "Load", Warn: "it costs", Uses: "bigquery"},
			{Name: "layers", Type: "list|string", Enum: []string{"bronze", "silver"}, Default: "bronze"},
			{Name: "since", Type: domain.ParamString, Pattern: `^\d{4}-\d{2}-\d{2}$`, Description: "the first day"},
		},
		Nodes: []domain.Node{
			{ID: "fetch", Run: "python fetch.py --full={{ .full }}", Runtime: "python", Tools: []string{"dbt"}, Env: map[string]string{"X": "1"},
				PersistContext: []string{"codes"}, Shell: &yes, Resources: domain.Resources{CPU: "1"}, Host: "license-box"},
			{ID: "load", Action: "docker.run", With: map[string]any{"image": "acme/load:1"},
				When: "all_done", OnError: &domain.OnError{Type: "SLACK", When: "give_up"}},
			{ID: "publish", Marker: true},
		},
		Edges: []domain.Edge{{From: "fetch", To: "load", Label: "rows"}, {From: "fetch", To: "publish"}, {From: "load", To: "publish"}},
	}
	roundTrip(t, "everything.yaml", dag)

	chain := domain.Workflow{Slug: "steps", Name: "steps", Kind: domain.KindChain, MaxActive: 3,
		Nodes: []domain.Node{{ID: "a", Run: "echo a"}, {ID: "b", Run: "echo b"}, {ID: "c", Run: "echo c"}},
		Edges: []domain.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}}}
	roundTrip(t, "steps.yaml", chain)
}

// THE YAML READS LIKE A FILE SOMEBODY WROTE: no empty keys, a debounce in
// its unit, a dependency with no label as its step's name.
func TestTheDefinitionsYAMLReadsLikeAFile(t *testing.T) {
	y, err := YAML(domain.Workflow{Slug: "w", Name: "w", Kind: domain.KindDAG,
		Trigger: domain.Trigger{OnLanded: []string{"gs://b/x"}, Debounce: 5 * time.Minute},
		Nodes:   []domain.Node{{ID: "a", Run: "echo a"}, {ID: "b", Run: "echo b"}},
		Edges:   []domain.Edge{{From: "a", To: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(y)
	for _, want := range []string{"name: w\n", "debounce: 5m0s", "depends_on:\n      - a\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("the YAML has no %q:\n%s", want, got)
		}
	}
	for _, empty := range []string{`description: ""`, "resources:", "env:", "image:", "params:"} {
		if strings.Contains(got, empty) {
			t.Errorf("the YAML carries an empty %s:\n%s", empty, got)
		}
	}
}
