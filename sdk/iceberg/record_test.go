package iceberg

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/sdk"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// one collects the single batch a small input produces, and keeps it alive
// past the loop: Records releases a batch once the loop body returns, so a
// test that wants to look at it afterwards has to Retain it -- which is the
// ownership contract, exercised.
func one(t *testing.T, s sdk.Schema, rows []sdk.Envelope) arrow.RecordBatch {
	t.Helper()
	var out arrow.RecordBatch
	for rec, err := range Records(s, rows) {
		if err != nil {
			t.Fatalf("Records: %v", err)
		}
		if out != nil {
			t.Fatal("a small input produced more than one batch")
		}
		rec.Retain()
		out = rec
	}
	if out == nil {
		t.Fatal("Records produced no batch")
	}
	t.Cleanup(out.Release)
	return out
}

func row(key string, fields map[string]any) sdk.Envelope {
	return sdk.Envelope{Provider: "test", Entity: "rows", SourceKey: key, RecordTS: "2026-11-02T10:00:00Z", Payload: fields}
}

// value reads one cell back in a shape a test can compare: decimals as text
// at the column's scale, instants as RFC 3339 in UTC, dates as YYYY-MM-DD.
func value(col arrow.Array, i int) any {
	if col.IsNull(i) {
		return nil
	}
	switch a := col.(type) {
	case *array.String:
		return a.Value(i)
	case *array.Int64:
		return a.Value(i)
	case *array.Float64:
		return a.Value(i)
	case *array.Decimal128:
		return a.Value(i).ToString(9)
	case *array.Boolean:
		return a.Value(i)
	case *array.Timestamp:
		return a.Value(i).ToTime(arrow.Microsecond).UTC().Format(time.RFC3339Nano)
	case *array.Date32:
		return a.Value(i).ToTime().Format("2006-01-02")
	case *array.Binary:
		return a.Value(i)
	}
	return fmt.Sprintf("unhandled %T", col)
}

// EACH TYPE CONVERTS FROM THE SHAPES A RECORD ACTUALLY ARRIVES IN, by the
// declared type and nothing else. A JSON record carries every number as a
// float64 and every instant as a string, so `int64` takes an integral float,
// `timestamp` takes RFC 3339, `numeric` takes text or a number, and `json`
// takes the document however it came. The column decides; the value never
// does (SDK_NEW_DRIVER.md, rule 2.5).
func TestEachTypeConvertsFromWhatARecordCarries(t *testing.T) {
	at := time.Date(2026, 11, 2, 10, 0, 0, 500_000, time.UTC)
	cases := []struct {
		typ  sdk.ColumnType
		in   []any
		want []any
	}{
		{sdk.TypeString, []any{"Coleen"}, []any{"Coleen"}},
		{sdk.TypeInt64,
			[]any{int64(7), 3, float64(42), json.Number("9"), uint8(2)},
			[]any{int64(7), int64(3), int64(42), int64(9), int64(2)}},
		{sdk.TypeFloat64,
			[]any{2.5, 3, json.Number("0.25"), float32(1.5)},
			[]any{2.5, 3.0, 0.25, 1.5}},
		// numeric is decimal(38, 9): text keeps what the source wrote, a
		// float is rounded to the scale, an integer is exact.
		{sdk.TypeNumeric,
			[]any{"1234.5", 2.25, 3, json.Number("0.000000001"), "-0.10"},
			[]any{"1234.500000000", "2.250000000", "3.000000000", "0.000000001", "-0.100000000"}},
		{sdk.TypeBool, []any{true, false}, []any{true, false}},
		// an instant keeps its zone's meaning and lands in UTC, at
		// microseconds, which is what Iceberg v2 stores.
		{sdk.TypeTimestamp,
			[]any{"2026-11-02T10:00:00.123456789-03:00", at, "2026-11-02T10:00:00Z"},
			[]any{"2026-11-02T13:00:00.123456Z", "2026-11-02T10:00:00.0005Z", "2026-11-02T10:00:00Z"}},
		{sdk.TypeDate,
			[]any{"2026-11-02", time.Date(2026, 11, 2, 23, 59, 0, 0, time.UTC)},
			[]any{"2026-11-02", "2026-11-02"}},
		// json is the serialized document: an object is marshalled, text
		// that already is a document goes as it is.
		{sdk.TypeJSON,
			[]any{map[string]any{"a": 1}, `{"b":[1,2]}`, json.RawMessage(`[1]`), []any{"x"}},
			[]any{`{"a":1}`, `{"b":[1,2]}`, `[1]`, `["x"]`}},
		// bytes arrive raw from Go and base64 from JSON, which is how
		// encoding/json writes them.
		{sdk.TypeBytes, []any{[]byte{1, 2}, "AQI="}, []any{[]byte{1, 2}, []byte{1, 2}}},
	}
	for _, c := range cases {
		t.Run(string(c.typ), func(t *testing.T) {
			rows := make([]sdk.Envelope, len(c.in))
			for i, v := range c.in {
				rows[i] = row(strconv.Itoa(i), map[string]any{"c": v})
			}
			rec := one(t, sdk.Schema{{Name: "c", Type: c.typ}}, rows)
			if rec.NumRows() != int64(len(c.in)) {
				t.Fatalf("%d rows, want %d", rec.NumRows(), len(c.in))
			}
			for i, want := range c.want {
				if got := value(rec.Column(0), i); !reflect.DeepEqual(got, want) {
					t.Errorf("row %d: %T %v in -> %v (%T), want %v", i, c.in[i], c.in[i], got, got, want)
				}
			}
		})
	}
}

// A VALUE THAT DOES NOT FIT IS REFUSED NAMING THE COLUMN, THE DECLARED TYPE
// AND WHAT ARRIVED, because the alternative -- a guess, a truncation, a
// silent null -- is data lost with nothing saying so. One refusal per type,
// plus the two that look like they could pass: a float with a fraction into
// `int64`, and a decimal with more digits than the scale holds.
func TestAValueThatDoesNotFitIsRefusedByName(t *testing.T) {
	cases := []struct {
		typ      sdk.ColumnType
		in       any
		received string // the Go type the message has to name
	}{
		{sdk.TypeString, 1, "int"},
		{sdk.TypeInt64, 3.5, "float64"},
		{sdk.TypeInt64, "7", "string"},
		{sdk.TypeFloat64, "x", "string"},
		{sdk.TypeNumeric, "abc", "string"},
		{sdk.TypeNumeric, "1.0123456789", "string"}, // ten decimals; the scale holds nine
		{sdk.TypeBool, "true", "string"},
		{sdk.TypeTimestamp, "02/11/2026 10:00", "string"},
		{sdk.TypeTimestamp, 1_762_000_000, "int"}, // seconds? milliseconds? refused rather than guessed
		{sdk.TypeDate, "2026-11-02T10:00:00Z", "string"},
		{sdk.TypeJSON, "{not json", "string"},
		{sdk.TypeBytes, "not base64!!", "string"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s from %s", c.typ, c.received), func(t *testing.T) {
			rows := []sdk.Envelope{
				row("ok", map[string]any{"amount": nil}),
				row("k-7", map[string]any{"amount": c.in}),
			}
			var err error
			for _, e := range Records(sdk.Schema{{Name: "amount", Type: c.typ}}, rows) {
				err = e
			}
			if err == nil {
				t.Fatalf("%v (%T) into a %s column: accepted", c.in, c.in, c.typ)
			}
			msg := err.Error()
			for _, want := range []string{`"amount"`, string(c.typ), c.received, `"k-7"`} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not name %s", msg, want)
				}
			}
		})
	}
}

// ABSENT AND NULL BOTH LAND AS NULL IN A COLUMN THAT TAKES IT, which is what a
// landing table does: a field the source sometimes does not send. In a
// required column the same two are a refusal naming the column, because a
// Parquet file with a null in a required field is a file half the readers
// reject later, with a worse message.
func TestAnAbsentOrNullValueIsNullUnlessTheColumnIsRequired(t *testing.T) {
	s := sdk.Schema{
		{Name: "id", Type: sdk.TypeString, Required: true},
		{Name: "amount", Type: sdk.TypeNumeric},
	}
	rec := one(t, s, []sdk.Envelope{
		row("a", map[string]any{"id": "a"}),
		row("b", map[string]any{"id": "b", "amount": nil}),
		row("c", map[string]any{"id": "c", "amount": "1.5"}),
	})
	amount := rec.Column(1)
	if !amount.IsNull(0) || !amount.IsNull(1) || amount.IsNull(2) {
		t.Errorf("amount nulls: %v %v %v, want absent and nil null, value not", amount.IsNull(0), amount.IsNull(1), amount.IsNull(2))
	}
	if got := value(amount, 2); got != "1.500000000" {
		t.Errorf("amount[2] = %v", got)
	}

	for name, fields := range map[string]map[string]any{
		"absent": {"amount": "1"},
		"null":   {"id": nil, "amount": "1"},
	} {
		var err error
		for _, e := range Records(s, []sdk.Envelope{row("x", fields)}) {
			err = e
		}
		if err == nil || !strings.Contains(err.Error(), `"id"`) || !strings.Contains(err.Error(), "required") {
			t.Errorf("%s required id: %v, want a refusal naming the column as required", name, err)
		}
	}
}

// A MILLION ROWS OF TEN COLUMNS GO THROUGH IN BATCHES OF BatchRows WITH THE
// HEAP UNDER 512 MB (spec criterion 8). The envelopes are the caller's and
// already in memory; what this measures is that the conversion streams --
// one batch alive at a time, released when the loop moves on -- instead of
// materializing a million Arrow rows before the first Parquet byte. The
// payloads cycle through a pool so the input itself stays small enough for
// the whole heap, input included, to fit the budget.
func TestAMillionRowsStreamInBatchesUnderTheMemoryBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("a million rows; skipped under -short")
	}
	const rows, budget = 1_000_000, 512 << 20
	s := sdk.Schema{
		{Name: "event_id", Type: sdk.TypeString, Required: true},
		{Name: "n", Type: sdk.TypeInt64},
		{Name: "ratio", Type: sdk.TypeFloat64},
		{Name: "amount", Type: sdk.TypeNumeric},
		{Name: "ok", Type: sdk.TypeBool},
		{Name: "occurred_at", Type: sdk.TypeTimestamp},
		{Name: "day", Type: sdk.TypeDate},
		{Name: "attrs", Type: sdk.TypeJSON},
		{Name: "blob", Type: sdk.TypeBytes},
		{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
	}
	pool := make([]map[string]any, 1024)
	for i := range pool {
		pool[i] = map[string]any{
			"event_id":            fmt.Sprintf("e-%06d", i),
			"n":                   float64(i),
			"ratio":               float64(i) / 7,
			"amount":              strconv.Itoa(i) + ".25",
			"ok":                  i%2 == 0,
			"occurred_at":         fmt.Sprintf("2026-11-%02dT%02d:00:00Z", i%28+1, i%24),
			"day":                 fmt.Sprintf("2026-11-%02d", i%28+1),
			"attrs":               map[string]any{"i": i, "tags": []any{"a", "b"}},
			"blob":                "AQIDBA==",
			"ingestion_loaded_at": "2026-11-02T12:00:00Z",
		}
	}
	envelopes := make([]sdk.Envelope, rows)
	for i := range envelopes {
		envelopes[i] = sdk.Envelope{SourceKey: strconv.Itoa(i), Payload: pool[i%len(pool)]}
	}

	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	peak, baseline := ms.HeapAlloc, ms.HeapAlloc

	var got, batches int64
	for rec, err := range Records(s, envelopes) {
		if err != nil {
			t.Fatal(err)
		}
		if rec.NumRows() > BatchRows {
			t.Fatalf("a batch of %d rows; the ceiling is %d", rec.NumRows(), BatchRows)
		}
		got += rec.NumRows()
		batches++
		runtime.ReadMemStats(&ms)
		peak = max(peak, ms.HeapAlloc)
	}
	if got != rows {
		t.Fatalf("%d rows came out, want %d", got, rows)
	}
	if want := int64((rows + BatchRows - 1) / BatchRows); batches != want {
		t.Errorf("%d batches, want %d", batches, want)
	}
	t.Logf("heap: %d MB before, %d MB at peak, budget %d MB", baseline>>20, peak>>20, budget>>20)
	if peak > budget {
		t.Errorf("heap peaked at %d MB; the budget is %d MB", peak>>20, budget>>20)
	}
}
