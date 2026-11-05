package postgres

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestToColumnRowByRow is the WRITE side of the type table.
//
// It exists because of a concrete defect: the SDK's record is JSON, so a
// timestamp in it is an RFC 3339 STRING. pgx's binary COPY wants a time.Time and
// refuses the string with "cannot find encode plan", which tells nobody the
// problem is the format. It only showed up against a real server -- the
// in-memory tests prove the bytes we assembled.
func TestToColumnRowByRow(t *testing.T) {
	instant := time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC)

	cases := []struct {
		name  string
		value any
		kind  string
		want  any
		why   string
	}{
		{"nil vira NULL em qualquer coluna", nil, "timestamp with time zone", nil, ""},
		{
			"string RFC 3339 vira time.Time",
			"2026-09-05T12:30:00Z", "timestamp with time zone", instant,
			"e a forma que IngestionLoadedAt produz, e a que toda API JSON devolve",
		},
		{
			"RFC 3339 com fracao",
			"2026-09-05T12:30:00.000Z", "timestamp with time zone", instant, "",
		},
		{
			"outro fuso vira o mesmo instante",
			"2026-09-05T09:30:00-03:00", "timestamp with time zone", instant, "",
		},
		{"time.Time passa direto", instant, "timestamp with time zone", instant, ""},
		{
			"epoch em segundos",
			float64(instant.Unix()), "timestamp with time zone", instant,
			"um JSON traz epoch como float64, e recusa-lo perderia a linha",
		},
		// numeric has a case of its own, in TestNumericGoesTypedAndNotAsText: it
		// comes out neither as a string nor as a float, but as a pgtype.Numeric
		// -- which preserves the precision AND avoids an error built per row
		// inside pgx.
		{"text passa como veio", "qualquer coisa", "text", "qualquer coisa", ""},
		{"integer passa como veio", int64(42), "integer", int64(42), ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := toColumn(c.value, c.kind)
			if err != nil {
				t.Fatalf("toColumn: %v", err)
			}
			if ts, ok := c.want.(time.Time); ok {
				gt, ok := got.(time.Time)
				if !ok || !gt.Equal(ts) {
					t.Errorf("= %#v, want %v\n  %s", got, ts, c.why)
				}
				return
			}
			if got != c.want {
				t.Errorf("= %#v, want %#v\n  %s", got, c.want, c.why)
			}
		})
	}
}

// TestToColumnSerializesJSON: a map in a jsonb column has to become
// a document, not Go's representation of a map.
func TestToColumnSerializesJSON(t *testing.T) {
	got, err := toColumn(map[string]any{"a": 1}, "jsonb")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got.(string)), &doc); err != nil {
		t.Fatalf("no valid JSON came out: %q", got)
	}
	if doc["a"] != float64(1) {
		t.Errorf("document = %v", doc)
	}
}

// TestToColumnTruncatesADate: a date column keeps no time, and sending a time
// the record did not have is inventing data.
func TestToColumnTruncatesADate(t *testing.T) {
	got, err := toColumn("2026-09-05T23:59:59Z", "date")
	if err != nil {
		t.Fatal(err)
	}
	ts := got.(time.Time)
	if ts.Hour() != 0 || ts.Minute() != 0 {
		t.Errorf("date = %v; the time should have been truncated", ts)
	}
}

// TestToColumnsErrorSaysTheFormat: "cannot find encode plan" tells nobody
// what to do. This error does.
func TestToColumnsErrorSaysTheFormat(t *testing.T) {
	_, err := toColumn("cinco de setembro", "timestamp with time zone")
	if err == nil {
		t.Fatal("text that is not a date got through")
	}
	for _, required := range []string{"RFC 3339", "2026-09-05T12:30:00Z"} {
		if !strings.Contains(err.Error(), required) {
			t.Errorf("the error does not say %q: %v", required, err)
		}
	}
}

// TestToColumnElidesALongValue: a 4 KB field inside an error message is
// noise, and it can carry data nobody wants in a log.
func TestToColumnElidesALongValue(t *testing.T) {
	long := strings.Repeat("x", 4000)
	_, err := toColumn(long, "date")
	if err == nil {
		t.Fatal("passou")
	}
	if len(err.Error()) > 300 {
		t.Errorf("the message has %d bytes; the value should have been elided", len(err.Error()))
	}
}

// TestNumericGoesTypedAndNotAsText pins a measured gain, so it does not
// come back in silence.
//
// Passando a string crua, o pgx tenta um plano de encode string->numeric, ele
// fails, and pgx builds an error just to fall through to the next plan -- once
// per
// row. On a 10-thousand-row load that was ~30% of the allocations, all in
// newEncodeError and fmt.Errorf: work spent producing an error nobody reads.
//
// Measured against the server: 290,529 allocations became 190,506, memory
// fell from 7.1 MB to 4.5 MB, and throughput rose from 352 thousand to ~434
// thousand
// rows/s.
func TestNumericGoesTypedAndNotAsText(t *testing.T) {
	got, err := toColumn("1234567890123456.78", "numeric")
	if err != nil {
		t.Fatal(err)
	}
	n, ok := got.(pgtype.Numeric)
	if !ok {
		t.Fatalf("toColumn returned %T; pgx encodes pgtype.Numeric directly and "+
			"pays for an error built per row for anything else", got)
	}
	if !n.Valid {
		t.Error("the number was not read")
	}

	// And the precision is still exact: that was the point of keeping it as
	// text.
	b, err := n.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1234567890123456.78" {
		t.Errorf("= %s, want the exact value", b)
	}
}

// TestAnInvalidNumericSaysWhatItGot, without dumping the whole value into the
// log.
func TestAnInvalidNumericSaysWhatItGot(t *testing.T) {
	if _, err := toColumn("dez reais", "numeric"); err == nil {
		t.Fatal("text that is not a number got through")
	}
	if _, err := toColumn(strings.Repeat("9", 4000)+"x", "numeric"); err != nil {
		if len(err.Error()) > 300 {
			t.Errorf("the message has %d bytes; the value should have been elided", len(err.Error()))
		}
	}
}

// TestANonTextualNumericPassesThrough: the server is what refuses, with a
// message of
// its own, which beats one of ours guessing.
func TestANonTextualNumericPassesThrough(t *testing.T) {
	got, err := toColumn(float64(10.5), "numeric")
	if err != nil {
		t.Fatal(err)
	}
	if got != float64(10.5) {
		t.Errorf("= %#v", got)
	}
}
