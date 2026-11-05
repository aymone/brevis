package sdk

import (
	"fmt"
	"strings"
	"testing"
	"time"

	core "brevis.sh/brevis/sdk/internal/core"
)

func applyTransform(t *testing.T, fn Transformer, in map[string]any) map[string]any {
	t.Helper()
	got, err := fn(in)
	if err != nil {
		t.Fatalf("transformer: %v", err)
	}
	return got.(map[string]any)
}

// The criterion that holds everything: the id has to be the one the block used
// to produce.
// If it changes, every previous load of every consumer stops matching.
//
// The expected value comes from Envelope.IngestionID, the implementation that
// came before and is still checked byte for byte against Python's uuid.uuid5.
func TestIngestionIDProducesTheSameIDAsBefore(t *testing.T) {
	env := core.Envelope{
		Provider: "open_meteo", Entity: "hourly_temperature",
		SourceKey: "-23.55|-46.63|2026-01-01T00:00", RecordTS: "2026-01-01T00:00",
	}
	expected, err := env.IngestionID()
	if err != nil {
		t.Fatal(err)
	}

	got := applyTransform(t, IngestionID(), map[string]any{
		"provider": "open_meteo", "entity": "hourly_temperature",
		"source_key": "-23.55|-46.63|2026-01-01T00:00", "record_ts": "2026-01-01T00:00",
	})

	if got[ColumnIngestionID] != expected {
		t.Errorf("the id changed formula:\n  transformer: %v\n  before:      %v",
			got[ColumnIngestionID], expected)
	}
}

// And against the frozen value, written out: a test comparing two
// implementations passes if both change together.
func TestIngestionIDAgainstTheFrozenValue(t *testing.T) {
	got := applyTransform(t, IngestionID(), map[string]any{
		"provider": "p", "entity": "e", "source_key": "k", "record_ts": "2026-01-01T00:00:00Z",
	})

	// Checked against Python's uuid.uuid5, which is the parity that matters:
	//   uuid.uuid5(UUID("e3a4f8c0-1b9d-4ea0-9c2e-77f6a6c4a4d7"),
	//              "p|e|k|2026-01-01T00:00:00Z")
	const frozen = "178d0b49-dece-5738-b8eb-f5cae222a1ea"
	if got[ColumnIngestionID] != frozen {
		t.Errorf("ingestion_id = %v, frozen at %s", got[ColumnIngestionID], frozen)
	}
}

func TestIngestionIDReadsTheNamesYouGiveIt(t *testing.T) {
	withCanonical := applyTransform(t, IngestionID(), map[string]any{
		"provider": "p", "entity": "e", "source_key": "k", "record_ts": "t",
	})
	withOthers := applyTransform(t, IngestionID("provider", "entity", "source_key", "time"),
		map[string]any{"provider": "p", "entity": "e", "source_key": "k", "time": "t"})

	if withCanonical[ColumnIngestionID] != withOthers[ColumnIngestionID] {
		t.Error("naming the fields must give the same id as the canonical ones")
	}
}

// A named-but-absent field is an error naming the field -- it usually means the
// chain is out of order, or that Without ran first.
func TestIngestionIDRefusesAMissingField(t *testing.T) {
	_, err := IngestionID()(map[string]any{"provider": "p", "entity": "e"})
	if err == nil {
		t.Fatal("missing source_key and record_ts, the id would be built from nothing")
	}
	for _, wants := range []string{"source_key", "record_ts"} {
		if !strings.Contains(err.Error(), wants) {
			t.Errorf("the error does not name %q: %v", wants, err)
		}
	}
	// And it says what the row does have, so the fix comes out of one reading.
	if !strings.Contains(err.Error(), "provider") {
		t.Errorf("the error does not list what the row has: %v", err)
	}
}

// An empty source_key is the case Envelope.IngestionID already refused: without
// it there is no stable identity, and the id would change on every run.
func TestIngestionIDRefusesAnEmptySourceKey(t *testing.T) {
	_, err := IngestionID()(map[string]any{
		"provider": "p", "entity": "e", "source_key": "", "record_ts": "t",
	})
	if err == nil {
		t.Fatal("an empty source_key gives no stable identity")
	}
}

func TestIngestionIDRefusesToOverwrite(t *testing.T) {
	_, err := IngestionID()(map[string]any{
		"provider": "p", "entity": "e", "source_key": "k", "record_ts": "t",
		"ingestion_id": "my-own",
	})
	if err == nil {
		t.Fatal("overwriting an id the row already has would be invisible")
	}
}

func TestIngestionIDRefusesTheWrongNumberOfFields(t *testing.T) {
	_, err := IngestionID("provider", "entity")(map[string]any{"provider": "p", "entity": "e"})
	if err == nil {
		t.Fatal("the formula has four components; two will not do")
	}
	if !strings.Contains(err.Error(), "4") {
		t.Errorf("the error must say how many: %v", err)
	}
}

func TestIngestionLoadedAtWritesNowInUTC(t *testing.T) {
	got := applyTransform(t, IngestionLoadedAt(), map[string]any{"a": 1})

	v, ok := got[ColumnIngestionLoadedAt].(string)
	if !ok {
		t.Fatalf("ingestion_loaded_at = %T", got[ColumnIngestionLoadedAt])
	}
	when, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("not RFC 3339: %q", v)
	}
	if d := time.Since(when); d > time.Minute || d < -time.Minute {
		t.Errorf("ingestion_loaded_at = %v, want now", when)
	}
	if !strings.HasSuffix(v, "Z") {
		t.Errorf("expected UTC: %q", v)
	}
	if got["a"] != 1 {
		t.Error("the rest of the row was lost")
	}
}

func TestIngestionLoadedAtRefusesToOverwrite(t *testing.T) {
	_, err := IngestionLoadedAt()(map[string]any{"ingestion_loaded_at": "yesterday"})
	if err == nil {
		t.Fatal("overwriting the load instant would be invisible")
	}
}

// TestTransformersWriteInPlace pins the NEW contract, and it is the opposite of
// the
// what this test used to assert.
//
// Each transformer returned a new map, "because the caller may still be
// holding the map". That is true once -- for the map the decoder handed over --
// and the other six copies per record were identical work repeated. The copy is
// now made once, in `applyAll`.
//
// Whoever calls a transformer ALONE, outside the chain, now sees their own row
// altered. It is documented on the Transformer type, and is the price of the
// arithmetic
// which the next test measures.
func TestTransformersWriteInPlace(t *testing.T) {
	line := map[string]any{"provider": "p", "entity": "e", "source_key": "k", "record_ts": "t"}
	output := applyTransform(t, IngestionID(), line)

	if _, ok := line[ColumnIngestionID]; !ok {
		t.Error("the transformer returned a new map; the chain's saving depends on it writing in place")
	}
	if fmt.Sprint(output) != fmt.Sprint(line) {
		t.Error("what came back is not the same map that went in")
	}
}

// TestTransformDoesNotMutateWhatExtractDelivered is the guarantee that came to
// matter, and that did not exist as a test before.
//
// The extract's preview keeps the record the SOURCE sent, in order to show
// exactly that. If the chain wrote over it, the preview would start
// show the Transform's result claiming it is the source's response -- a lie
// nobody would have any way to notice.
func TestTransformDoesNotMutateWhatExtractDelivered(t *testing.T) {
	original := map[string]any{"provider": "p", "entity": "e", "source_key": "k", "record_ts": "t"}

	output, skipped, err := applyAll([]Transformer{IngestionID(), IngestionLoadedAt()}, original)
	if err != nil || skipped {
		t.Fatalf("applyAll: %v, skipped=%v", err, skipped)
	}

	if len(original) != 4 {
		t.Errorf("the chain wrote into the extract's record: %v", original)
	}
	obj := output.(map[string]any)
	if len(obj) != 6 {
		t.Errorf("the output lacks the two new columns: %v", obj)
	}
}

// TestTheChainMakesOneCopyOnly is the arithmetic that justifies the change.
func TestTheChainMakesOneCopyOnly(t *testing.T) {
	fns := []Transformer{
		Accept("provider", "entity", "source_key", "record_ts"),
		Rename(map[string]string{"record_ts": "ts"}),
		Compute("extra", func(map[string]any) (any, error) { return 1, nil }),
		Without("extra"),
	}

	allocations := testing.AllocsPerRun(200, func() {
		line := map[string]any{"provider": "p", "entity": "e", "source_key": "k", "record_ts": "t", "junk": 1}
		if _, _, err := applyAll(fns, line); err != nil {
			t.Fatal(err)
		}
	})

	// The input row costs one map, the chain's copy costs another. Four
	// transformers that each copied would cost four more -- and it is that
	// difference the ceiling catches, not an absolute allocation count.
	const ceiling float64 = 12
	if allocations > ceiling {
		t.Errorf("%.0f allocations for one row and four transformers (ceiling %.0f); "+
			"some transformer went back to copying the map", allocations, ceiling)
	}
	t.Logf("%.0f allocations", allocations)
}
