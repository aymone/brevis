package sdk_test

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"brevis.sh/brevis/sdk"
	"brevis.sh/brevis/sdk/from"
)

// TestTheSnapshotDoesNotDependOnThePositionInTheChain is the reason the
// snapshot is NOT a transformer.
//
// As a transformer, the snapshot would depend on position: placing it after a
// Compute would produce a "raw" record carrying the field the chain had just
// writing it. That produces no error -- it produces wrong data nobody notices
// until somebody queries it months later.
//
// Taken where the record leaves the source, no ordering can contaminate it.
func TestTheSnapshotDoesNotDependOnThePositionInTheChain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":"1","temperatura":19.5}`)
	}))
	defer srv.Close()

	data, err := sdk.Extract(context.Background(), sdk.Source{
		From: from.HTTP{URL: srv.URL}, Snapshot: "payload",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The chain writes AFTER the snapshot, and writes plenty.
	data = sdk.Transform(data,
		sdk.Compute("derivado", func(map[string]any) (any, error) { return "novo", nil }),
		sdk.Rename(map[string]string{"id": "source_key"}),
	)

	var line map[string]any
	for env, err := range data.Records {
		if err != nil {
			t.Fatal(err)
		}
		line = env.Payload.(map[string]any)
	}

	snapshot, ok := line["payload"].(map[string]any)
	if !ok {
		t.Fatalf("no snapshot: %v", line)
	}
	if _, tainted := snapshot["derivado"]; tainted {
		t.Errorf("the snapshot carries a field the chain wrote: %v", snapshot)
	}
	if snapshot["id"] != "1" {
		t.Errorf("the snapshot lost the field's original name: %v", snapshot)
	}
	if line["source_key"] != "1" {
		t.Errorf("the chain did not run on the record: %v", line)
	}
}

// TestTheSnapshotRefusesToOverwriteWhatTheSourceSent: writing over it would
// lose the
// that came from the source, in silence.
func TestTheSnapshotRefusesToOverwriteWhatTheSourceSent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"payload":"da fonte"}`)
	}))
	defer srv.Close()

	data, err := sdk.Extract(context.Background(), sdk.Source{
		From: from.HTTP{URL: srv.URL}, Snapshot: "payload",
	})
	if err != nil {
		t.Fatal(err)
	}
	var seen error
	for _, err := range data.Records {
		if err != nil {
			seen = err
		}
	}
	if seen == nil {
		t.Fatal("wrote the snapshot over a field of the source")
	}
	if !strings.Contains(seen.Error(), "payload") {
		t.Errorf("the error does not name the field: %v", seen)
	}
}

// TestSkipWithoutDropsInsteadOfFailing: a row without the field that composes
// the key has no stable identity and cannot go in -- but it is also no reason
// to
// derrubar a janela inteira.
func TestSkipWithoutDropsInsteadOfFailing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[{"id":"1"},{"id":null},{"outro":2},{"id":"4"}]`)
	}))
	defer srv.Close()

	data, err := sdk.Extract(context.Background(), sdk.Source{From: from.HTTP{URL: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	data = sdk.Transform(data, sdk.SkipWithout("id"))

	var ids []string
	for env, err := range data.Records {
		if err != nil {
			t.Fatalf("SkipWithout derrubou a janela: %v", err)
		}
		ids = append(ids, env.Payload.(map[string]any)["id"].(string))
	}
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "4" {
		t.Errorf("%v left over; expected both with an id -- null and absent are the same thing here", ids)
	}
}

// TestTheNamespaceChangesTheID is item 1: the namespace belongs to whoever uses
// it, not to the
// biblioteca.
func TestTheNamespaceChangesTheID(t *testing.T) {
	record := func() map[string]any {
		return map[string]any{
			"provider": "acme", "entity": "pedidos",
			"source_key": "1", "record_ts": "2026-09-05T12:00:00Z",
		}
	}

	fallback, err := sdk.IngestionID()(record())
	if err != nil {
		t.Fatal(err)
	}
	mine := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	another, err := sdk.Namespace(mine).IngestionID()(record())
	if err != nil {
		t.Fatal(err)
	}

	defaultID := fallback.(map[string]any)[sdk.ColumnIngestionID].(string)
	otherID := another.(map[string]any)[sdk.ColumnIngestionID].(string)
	if defaultID == otherID {
		t.Error("different namespaces produced the same id; so the namespace is not used")
	}
}

// TestTheDefaultNamespaceHasNotChanged is the guarantee that stops the feature
// from breaking whoever has already written: the id for somebody who chooses no
// namespace has to be byte for byte the one from before.
func TestTheDefaultNamespaceHasNotChanged(t *testing.T) {
	output, err := sdk.IngestionID()(map[string]any{
		"provider": "open_meteo", "entity": "hourly",
		"source_key": "123", "record_ts": "2026-09-05T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := output.(map[string]any)[sdk.ColumnIngestionID].(string)

	// This value has been the same since v0.1.x, checked against Python's
	// uuid.uuid5. If it changes, every row already written has lost its
	// identity.
	const frozen = "7e18f9f9-37c4-5033-abce-def940db4cba"
	if got != frozen {
		t.Errorf("the default id changed: %s, was %s", got, frozen)
	}
}

// TestAChosenNamespaceIsDeterministic: choosing a namespace must not introduce
// variation between runs.
func TestAChosenNamespaceIsDeterministic(t *testing.T) {
	mine := sdk.Namespace(uuid.MustParse("11111111-2222-3333-4444-555555555555"))
	var previous string
	for i := 0; i < 5; i++ {
		output, err := mine.IngestionID()(map[string]any{
			"provider": "a", "entity": "b", "source_key": "c", "record_ts": "d",
		})
		if err != nil {
			t.Fatal(err)
		}
		got := output.(map[string]any)[sdk.ColumnIngestionID].(string)
		if i > 0 && got != previous {
			t.Fatalf("the id varied between runs: %s and %s", previous, got)
		}
		previous = got
	}
}

// countingTarget records every batch it receives.
type countingTarget struct {
	batches [][]int
	failAt  int // > 0: a leva N falha
}

func (d *countingTarget) Describe() string { return "destino de teste" }

func (d *countingTarget) Write(_ context.Context, envs []sdk.Envelope, _ sdk.WriteOptions) (*sdk.LoadResult, error) {
	var ids []int
	for _, e := range envs {
		ids = append(ids, e.Payload.(map[string]any)["i"].(int))
	}
	d.batches = append(d.batches, ids)
	if d.failAt > 0 && len(d.batches) == d.failAt {
		return &sdk.LoadResult{RowsLoaded: 0}, fmt.Errorf("batch %d failed", d.failAt)
	}
	return &sdk.LoadResult{RowsLoaded: int64(len(envs))}, nil
}

// TestFlushEveryWritesInBatches: a long read must not have the whole batch alive
// in memory, and the destination builds a second copy of it to serialize.
func TestFlushEveryWritesInBatches(t *testing.T) {
	target := &countingTarget{}
	res, err := sdk.Load(context.Background(), dataOf(t, 10), sdk.Target{
		To: target, FlushEvery: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(target.batches) != 4 {
		t.Fatalf("%d batches, want 4 (3+3+3+1)", len(target.batches))
	}
	if len(target.batches[3]) != 1 {
		t.Errorf("the last batch has %d, want 1", len(target.batches[3]))
	}

	// The Result sums the batches. A Rows counting only the last one would lie
	// to whoever reads the pipeline's line.
	if res.Rows != 10 || res.Records != 10 {
		t.Errorf("Rows=%d Records=%d, want 10 and 10", res.Rows, res.Records)
	}
}

// TestFlushEveryZeroAccumulatesEverything: the default does not change.
func TestFlushEveryZeroAccumulatesEverything(t *testing.T) {
	target := &countingTarget{}
	if _, err := sdk.Load(context.Background(), dataOf(t, 10), sdk.Target{To: target}); err != nil {
		t.Fatal(err)
	}
	if len(target.batches) != 1 || len(target.batches[0]) != 10 {
		t.Errorf("batches = %v; without FlushEvery the load is a single one", target.batches)
	}
}

// TestFlushEveryFailingMidwaySaysWhatWentIn: the load stops being atomic, and
// hiding that the previous batches were written would be worse than saying so
// -- whoever
// re-runs it needs to know that 6 rows are already there.
func TestFlushEveryFailingMidwaySaysWhatWentIn(t *testing.T) {
	target := &countingTarget{failAt: 3}
	res, err := sdk.Load(context.Background(), dataOf(t, 10), sdk.Target{
		To: target, FlushEvery: 3,
	})
	if err == nil {
		t.Fatal("the batch failed and the Load succeeded")
	}
	if res == nil {
		t.Fatal("the Result did not come back; whoever reruns would not know what already went in")
	}
	if res.Rows != 6 {
		t.Errorf("Rows = %d, want 6 -- the first two batches wrote", res.Rows)
	}
}

func dataOf(t *testing.T, n int) *sdk.Data {
	t.Helper()
	data, err := sdk.Extract(context.Background(), sdk.Source{From: sourceOfN{n}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type sourceOfN struct{ n int }

func (sourceOfN) Describe() string { return "fonte de teste" }
func (f sourceOfN) Read(context.Context, sdk.ReadOptions) (iter.Seq2[sdk.Envelope, error], error) {
	return func(yield func(sdk.Envelope, error) bool) {
		for i := 0; i < f.n; i++ {
			if !yield(sdk.Envelope{Payload: map[string]any{"i": i}}, nil) {
				return
			}
		}
	}, nil
}
