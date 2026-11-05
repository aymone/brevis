package sdk

import (
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"runtime"
	"strings"
	"testing"
)

func linesOf(records ...map[string]any) iter.Seq2[Envelope, error] {
	return func(yield func(Envelope, error) bool) {
		for _, r := range records {
			if !yield(Envelope{Payload: r}, nil) {
				return
			}
		}
	}
}

func runReduce(t *testing.T, d *Reduce, records ...map[string]any) []map[string]any {
	t.Helper()
	if err := d.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	var out []map[string]any
	for env, err := range d.apply(linesOf(records...)) {
		if err != nil {
			t.Fatalf("reduce: %v", err)
		}
		out = append(out, env.Payload.(map[string]any))
	}
	return out
}

func sales() []map[string]any {
	return []map[string]any{
		{"region": "south", "year": 2025.0, "value": 10.0, "name": "a", "ok": true},
		{"region": "south", "year": 2026.0, "value": 30.0, "name": "b", "ok": false},
		{"region": "north", "year": 2026.0, "value": 20.0, "name": "c", "ok": true},
		{"region": "south", "year": 2024.0, "value": nil, "name": "d", "ok": true},
	}
}

func TestAggregatorsComputeWhatTheyPromise(t *testing.T) {
	lines := runReduce(t, &Reduce{
		By: GroupBy("region"),
		Agg: map[string]Aggregator{
			"lines":      Count(),
			"with_value": CountOf("value"),
			"total":      Sum("value"),
			"mean":       Mean("value"),
			"min":        Min("value"),
			"max":        Max("value"),
			"first":      First("name"),
			"last":       Last("name"),
			"name_final": MaxBy("name", "year"),
			"name_old":   MinBy("name", "year"),
			"range":      Range("value"),
			"any_ok":     Any("ok"),
			"all_ok":     All("ok"),
		},
	}, sales()...)

	if len(lines) != 2 {
		t.Fatalf("came out with %d groups, want 2", len(lines))
	}
	// Deterministic order by key: "norte" before "sul".
	if lines[0]["region"] != "north" || lines[1]["region"] != "south" {
		t.Fatalf("non-deterministic order: %v", lines)
	}

	secondLine := lines[1]
	cases := map[string]any{
		"lines": int64(3), "with_value": int64(2),
		"total": 40.0, "mean": 20.0, "min": 10.0, "max": 30.0,
		"first": "a", "last": "d",
		"name_final": "b", "name_old": "d",
		"range": 20.0, "any_ok": true, "all_ok": false,
	}
	for k, want := range cases {
		if secondLine[k] != want {
			t.Errorf("%s = %v (%T), want %v", k, secondLine[k], secondLine[k], want)
		}
	}
}

// A group with no values returns null, not zero. Zero is a number somebody will
// add up; null says there was nothing to add.
func TestAGroupWithNoValuesReturnsNullNotZero(t *testing.T) {
	lines := runReduce(t, &Reduce{
		By:  GroupBy("g"),
		Agg: map[string]Aggregator{"total": Sum("v"), "mean": Mean("v"), "n": Count()},
	}, map[string]any{"g": "x", "v": nil})

	if lines[0]["total"] != nil || lines[0]["mean"] != nil {
		t.Errorf("total=%v avg=%v, both should be null", lines[0]["total"], lines[0]["mean"])
	}
	if lines[0]["n"] != int64(1) {
		t.Errorf("the row existed and Count should see it: %v", lines[0]["n"])
	}
}

// Welford: the naive formula loses every digit on large, close values, and
// returns a NEGATIVE variance.
func TestVarianceSurvivesLargeValues(t *testing.T) {
	base := 1e9
	var records []map[string]any
	for _, d := range []float64{0, 1, 2, 3, 4} {
		records = append(records, map[string]any{"g": "x", "v": base + d})
	}
	lines := runReduce(t, &Reduce{
		By:  GroupBy("g"),
		Agg: map[string]Aggregator{"var": Variance("v"), "dp": StdDev("v")},
	}, records...)

	v := lines[0]["var"].(float64)
	if v < 0 {
		t.Fatalf("negative variance (%v): the formula lost the digits", v)
	}
	if math.Abs(v-2.5) > 1e-6 {
		t.Errorf("variance = %v, want 2.5", v)
	}
	if dp := lines[0]["dp"].(float64); math.Abs(dp-math.Sqrt(2.5)) > 1e-9 {
		t.Errorf("deviation = %v", dp)
	}
}

// A field with the wrong name would produce a column of nulls, and nobody would
// notice. It is refused, naming what does exist.
func TestAFieldNoRowHasIsRefused(t *testing.T) {
	d := &Reduce{By: GroupBy("region"), Agg: map[string]Aggregator{"total": Sum("vlaue")}}
	var failure error
	for _, err := range d.apply(linesOf(sales()...)) {
		if err != nil {
			failure = err
		}
	}
	if failure == nil {
		t.Fatal("a nonexistent field passed as a column of nulls")
	}
	if !strings.Contains(failure.Error(), "vlaue") || !strings.Contains(failure.Error(), "value") {
		t.Errorf("the message must name the wrong one and list the right ones: %v", failure)
	}
}

// Mixed types in the same field are an error: the ordering between 10 and "9"
// would depend on arrival order, and the maximum would change between runs.
func TestMixedTypesAreAnError(t *testing.T) {
	d := &Reduce{By: GroupBy("g"), Agg: map[string]Aggregator{"max": Max("v")}}
	var failure error
	for _, err := range d.apply(linesOf(
		map[string]any{"g": "x", "v": 10.0},
		map[string]any{"g": "x", "v": "9"},
	)) {
		if err != nil {
			failure = err
		}
	}
	if failure == nil {
		t.Fatal("mixing number and text got through")
	}
	if !strings.Contains(failure.Error(), "arrival order") {
		t.Errorf("the message does not say the problem: %v", failure)
	}
}

// Text that is a number IS a number: a CSV hands everything over as text, and
// refusing it would force a transformer just to convert.
func TestNumericTextAddsUp(t *testing.T) {
	lines := runReduce(t, &Reduce{
		By:  GroupBy("g"),
		Agg: map[string]Aggregator{"total": Sum("v")},
	},
		map[string]any{"g": "x", "v": "12.5"},
		map[string]any{"g": "x", "v": json.Number("2.5")},
	)
	if lines[0]["total"] != 15.0 {
		t.Errorf("total = %v", lines[0]["total"])
	}
}

// GroupBy() with no fields reduces the whole stream to one row -- the grand
// total.
func TestGroupByWithNoFieldsGivesTheGrandTotal(t *testing.T) {
	lines := runReduce(t, &Reduce{
		Agg: map[string]Aggregator{"total": Sum("value"), "n": Count()},
	}, sales()...)
	if len(lines) != 1 {
		t.Fatalf("came out with %d rows, want 1", len(lines))
	}
	if lines[0]["total"] != 60.0 || lines[0]["n"] != int64(4) {
		t.Errorf("wrong grand total: %v", lines[0])
	}
}

// Finish sees the GROUPS, not the records -- it is what allows the global phase
// without undoing the memory guarantee.
func TestFinishSeesTheGroups(t *testing.T) {
	lines := runReduce(t, &Reduce{
		By:  GroupBy("region"),
		Agg: map[string]Aggregator{"total": Sum("value")},
		Finish: func(groups iter.Seq2[Group, map[string]any]) ([]map[string]any, error) {
			var best map[string]any
			for g, row := range groups {
				if len(g.Fields) != 1 || g.Fields[0] != "region" {
					return nil, fmt.Errorf("the group did not bring its fields: %v", g.Fields)
				}
				if best == nil || row["total"].(float64) > best["total"].(float64) {
					best = row
				}
			}
			return []map[string]any{best}, nil
		},
	}, sales()...)

	if len(lines) != 1 || lines[0]["region"] != "south" {
		t.Errorf("the global reduction did not pick the largest: %v", lines)
	}
}

// The refusals fail at ASSEMBLY time, before the extraction: finding them
// afterwards
// would cost the provider's window.
func TestWhatDoesNotFitRefusesBeforeExtracting(t *testing.T) {
	for name, a := range map[string]Aggregator{
		"Median":   Median("v"),
		"Quantile": Quantile("v", 0.9),
		"Distinct": Distinct("v"),
		"Mode":     Mode("v"),
		"Collect":  Collect("v"),
	} {
		err := (&Reduce{By: GroupBy("g"), Agg: map[string]Aggregator{"x": a}}).validate()
		if err == nil {
			t.Errorf("%s passed validation", name)
			continue
		}
		// The message has to name both ways out, because both exist.
		for _, expected := range []string{"constant", "SQL", "sdk.Custom"} {
			if !strings.Contains(err.Error(), expected) {
				t.Errorf("%s: the message does not say %q: %v", name, expected, err)
			}
		}
	}
}

func TestANameCollidingWithTheGroupIsRefused(t *testing.T) {
	err := (&Reduce{
		By:  GroupBy("region"),
		Agg: map[string]Aggregator{"region": Count()},
	}).validate()
	if err == nil || !strings.Contains(err.Error(), "two values") {
		t.Errorf("a name collision got through: %v", err)
	}
}

// --- The proof that matters ------------------------------------------------

// generateGroups produces n records spread over g groups, materializing
// nothing: the source has to fit in a stream, or the test measures itself.
func generateGroups(n, g int) iter.Seq2[Envelope, error] {
	return func(yield func(Envelope, error) bool) {
		for i := 0; i < n; i++ {
			r := map[string]any{
				"group": fmt.Sprintf("g%04d", i%g),
				"value": float64(i % 1000),
				"name":  "line",
			}
			if !yield(Envelope{Payload: r}, nil) {
				return
			}
		}
	}
}

// heapPeak measures the largest live heap DURING the fold.
//
// Measuring afterwards does not work, and that cost one version of this test:
// by the time the first row comes out, the groups' state is already dead -- the
// fold produced the output and Go collects the rest -- so the `GC` erased
// exactly what was to be measured, and an aggregator keeping a million rows
// passed with 3 MB.
//
// The probe goes in as one more aggregator: its `Add` runs once per
// record, while ALL of the groups' state is alive.
func heapPeak(t *testing.T, n, groups int, agg map[string]Aggregator) uint64 {
	t.Helper()

	var peak uint64
	var i int
	interval := n / 20
	if interval < 1 {
		interval = 1
	}

	withProbe := make(map[string]Aggregator, len(agg)+1)
	for k, v := range agg {
		withProbe[k] = v
	}
	withProbe["_probe"] = Custom(Accumulator{
		Init: func() any { return nil },
		Add: func(any, map[string]any) error {
			i++
			if i%interval != 0 {
				return nil
			}
			// A GC before reading: without it the measurement would be
			// dominated by the garbage of the
			// records already processed, which is noise, not retention.
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			return nil
		},
		Value: func(any) (any, error) { return nil, nil },
	})

	d := &Reduce{By: GroupBy("group"), Agg: withProbe}
	var lines int
	for _, err := range d.apply(generateGroups(n, groups)) {
		if err != nil {
			t.Fatal(err)
		}
		lines++
	}
	if lines != groups {
		t.Fatalf("came out with %d groups, want %d", lines, groups)
	}
	return peak
}

// Reduce's promise is not "the sum is right": it is that MEMORY does not grow
// with the input.
//
// A hundred fixed groups, the input growing 100x. If the ceiling holds, the promise
// is in the code and not only in the documentation -- and this is the feature's
// most likely regression: somebody adding an aggregator that keeps rows.
func TestMemoryDoesNotGrowWithTheInput(t *testing.T) {
	if testing.Short() {
		t.Skip("measures heap with 1M records")
	}
	const groups = 100
	agg := map[string]Aggregator{
		"n": Count(), "total": Sum("value"), "mean": Mean("value"),
		"max": Max("value"), "dp": StdDev("value"), "name": MaxBy("name", "value"),
	}

	small := heapPeak(t, 10_000, groups, agg)
	big := heapPeak(t, 1_000_000, groups, agg)

	// A fixed allowance, not a proportional one: if the input were retained, 1M
	// records would weigh hundreds of MB and blow past any reasonable
	// allowance.
	const slack = 4 << 20
	if big > small+slack {
		t.Errorf("the heap grew with the input: %.1f MB with 10 thousand records, "+
			"%.1f MB with 1 million. Memory should depend only on the %d groups",
			mb(small), mb(big), groups)
	}
}

// And the measurement above is only worth anything if it can CATCH the defect.
// This aggregator keeps the rows -- exactly what the rule forbids -- and the
// same ceiling has to fail it.
func TestTheMeasurementCatchesAnAggregatorThatKeepsRows(t *testing.T) {
	if testing.Short() {
		t.Skip("mede heap")
	}
	const groups = 100
	guardedAgg := map[string]Aggregator{
		"everything": Custom(Accumulator{
			Init: func() any { return &[]map[string]any{} },
			Add: func(acc any, r map[string]any) error {
				p := acc.(*[]map[string]any)
				*p = append(*p, r)
				return nil
			},
			Value: func(acc any) (any, error) { return len(*acc.(*[]map[string]any)), nil },
		}),
	}

	small := heapPeak(t, 10_000, groups, guardedAgg)
	big := heapPeak(t, 200_000, groups, guardedAgg)

	const slack = 4 << 20
	if big <= small+slack {
		t.Errorf("the measurement did NOT catch an aggregator that keeps rows: "+
			"%.1f MB against %.1f MB. The promise's test is worthless this way",
			mb(small), mb(big))
	}
}

func mb(b uint64) float64 { return float64(b) / (1 << 20) }
