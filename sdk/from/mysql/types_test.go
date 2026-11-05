package mysql

import (
	"encoding/json"
	"testing"
	"time"
)

// TestToJSONRowByRow covers the type table, one case per line.
//
// database/sql returns []byte for nearly everything when read into an `any`, so
// the
// conversion comes from the column's DECLARED TYPE. Without it every DECIMAL
// would become
// base64 in the JSON, and so would an INT.
func TestToJSONRowByRow(t *testing.T) {
	cases := []struct {
		name     string
		value    any
		declared string
		want     string
		why      string
	}{
		{"NULL", nil, "VARCHAR", `null`, ""},
		{
			"DECIMAL becomes string", []byte("123456789012345678.99"), "DECIMAL",
			`"123456789012345678.99"`,
			"float64 loses cents on large values",
		},
		{"BIGINT becomes number", []byte("42"), "BIGINT", `42`, "without this it would become base64"},
		{"INT becomes number", []byte("7"), "INT", `7`, ""},
		{"DOUBLE becomes number", []byte("1.5"), "DOUBLE", `1.5`, ""},
		{"VARCHAR becomes text", []byte("hello"), "VARCHAR", `"hello"`, ""},
		{
			"nested JSON", []byte(`{"a":[1,2]}`), "JSON", `{"a":[1,2]}`,
			"reserializing would become a string with JSON inside",
		},
		{
			"invalid JSON becomes string", []byte(`{broken`), "JSON", `"{broken"`,
			"the data exists; refusing would lose the whole record over one column",
		},
		{"BLOB in base64", []byte{0xde, 0xad}, "BLOB", `"3q0="`, ""},
		{"VARBINARY in base64", []byte{0xde, 0xad}, "VARBINARY", `"3q0="`, ""},
		{"DATE as text", []byte("2026-09-05"), "DATE", `"2026-09-05"`, ""},
		{
			"DATETIME without parseTime", []byte("2026-09-05 12:30:00"), "DATETIME",
			`"2026-09-05T12:30:00Z"`,
			"this is the path that exists when the DSN has no parseTime",
		},
		{"ENUM becomes text", []byte("active"), "ENUM", `"active"`, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(ToJSON(c.value, c.declared))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != c.want {
				t.Errorf("= %s, want %s\n  %s", b, c.want, c.why)
			}
		})
	}
}

// TestTheTwoInstantPathsAgree is the reason the text fallback exists, and the
// proof that it is not dead code: with or without parseTime, the same instant
// comes out the same.
//
// This test also prevents the lying comment that had been written -- that
// without parseTime the instant would become base64. It does not; what changes
// is the cost.
func TestTheTwoInstantPathsAgree(t *testing.T) {
	instant := time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC)

	withParse := ToJSON(instant, "DATETIME")
	noParse := ToJSON([]byte("2026-09-05 12:30:00"), "DATETIME")

	if withParse != noParse {
		t.Errorf("the paths diverge: with parseTime %v, without %v", withParse, noParse)
	}
	if withParse != "2026-09-05T12:30:00Z" {
		t.Errorf("= %v", withParse)
	}
}

// TestADateGainsNoTime: 00:00:00 is a time nobody wrote, and it walks a day on
// the first timezone conversion.
func TestADateGainsNoTime(t *testing.T) {
	d := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	if got := ToJSON(d, "DATE"); got != "2026-09-05" {
		t.Errorf("DATE = %v, expected no time", got)
	}
}
