package postgres

import (
	"encoding/json"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestToJSONRowByRow covers §3.1's table from the plan, one case per line.
//
// It is not inference: it is a written, reviewable table, and every choice here
// has
// a concrete cost when it is wrong -- which is why each case says what it is.
func TestToJSONRowByRow(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string // the JSON that comes out
		why   string
	}{
		{
			"NULL becomes null", nil, `null`,
			"nil is absence; any other value would invent data",
		},
		{
			"NUMERIC becomes string",
			pgtype.Numeric{Int: big.NewInt(123456789012345678), Exp: -2, Valid: true},
			`"1234567890123456.78"`,
			"float64 loses cents on large values, and the loss shows up months later",
		},
		{
			"negative NUMERIC",
			pgtype.Numeric{Int: big.NewInt(-1050), Exp: -2, Valid: true},
			`"-10.50"`,
			"",
		},
		{
			"null NUMERIC", pgtype.Numeric{}, `null`, "",
		},
		{
			"DATE with no time",
			pgtype.Date{Time: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), Valid: true},
			`"2026-09-05"`,
			"00:00:00 is a time nobody wrote, and it moves a day when converted across zones",
		},
		{
			"TIMESTAMPTZ in RFC 3339",
			pgtype.Timestamptz{Time: time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC), Valid: true},
			`"2026-09-05T12:30:00Z"`,
			"",
		},
		{
			"TIMESTAMPTZ in another zone becomes UTC",
			pgtype.Timestamptz{
				Time:  time.Date(2026, 9, 5, 9, 30, 0, 0, time.FixedZone("BRT", -3*3600)),
				Valid: true,
			},
			`"2026-09-05T12:30:00Z"`,
			"two records of the same instant in different zones must come out equal",
		},
		{
			"BYTEA in base64", []byte{0xde, 0xad, 0xbe, 0xef}, `"3q2+7w=="`,
			"encoding/json already does it, and the alternative would be an array of numbers",
		},
		{
			"nested JSONB",
			json.RawMessage(`{"a":[1,2],"b":{"c":true}}`),
			`{"a":[1,2],"b":{"c":true}}`,
			"reserializing would become a string with JSON inside, and the consumer would decode twice",
		},
		{
			"invalid JSONB becomes string",
			json.RawMessage(`{this is not json`),
			`"{this is not json"`,
			"the data exists; refusing here would lose the whole record because of one column",
		},
		{
			"UUID as text",
			pgtype.UUID{Bytes: [16]byte{0x17, 0x8d, 0x0b, 0x49, 0xde, 0xce, 0x57, 0x38,
				0xb8, 0xeb, 0xf5, 0xca, 0xe2, 0x22, 0x1a, 0xea}, Valid: true},
			`"178d0b49-dece-5738-b8eb-f5cae2221aea"`,
			"raw it would become an array of 16 numbers",
		},
		{
			"null UUID", pgtype.UUID{}, `null`, "",
		},
		{
			"array becomes array",
			[]any{int64(1), int64(2), nil},
			`[1,2,null]`,
			"",
		},
		{
			"NUMERIC array converts element by element",
			[]any{pgtype.Numeric{Int: big.NewInt(150), Exp: -2, Valid: true}},
			`["1.50"]`,
			"without recursion, the element would come out as a pgtype object",
		},
		{
			"INET as text", net.ParseIP("10.0.0.1"), `"10.0.0.1"`, "",
		},
		{
			"text passes straight through", "hello", `"hello"`, "",
		},
		{
			"integer passes straight through", int64(42), `42`, "",
		},
		{
			"boolean passes straight through", true, `true`, "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(ParaJSON(c.input))
			if err != nil {
				t.Fatalf("serializando: %v", err)
			}
			if string(b) != c.want {
				msg := "ParaJSON(%v) = %s, want %s"
				if c.why != "" {
					msg += "\n  " + c.why
				}
				t.Errorf(msg, c.input, b, c.want)
			}
		})
	}
}

// TestNumericLosesNoPrecision is the case that motivates the most important row
// of the
// table: a value a float64 cannot represent.
func TestNumericLosesNoPrecision(t *testing.T) {
	// 9007199254740993 = 2^53 + 1, the first integer a float64 cannot hold.
	n := pgtype.Numeric{Int: big.NewInt(9007199254740993), Exp: 0, Valid: true}

	got := ParaJSON(n)
	if got != "9007199254740993" {
		t.Errorf("ParaJSON = %v (%T), want a exact string", got, got)
	}

	// And the proof of the opposite, measured at run time: through a float64 the
	// value changes. Written with a variable and not a constant on purpose --
	// as a constant Go compares at compile time with arbitrary
	// precision, and the demonstration is always true.
	exact := int64(9007199254740993)
	byFloat := int64(float64(exact))
	if byFloat == exact {
		t.Fatal("float64 represented 2^53+1 on this platform; the choice of string needs " +
			"to be revisited, or this test no longer proves what it says")
	}
	if b, _ := json.Marshal(float64(exact)); string(b) == "9007199254740993" {
		t.Error("the float64 path did not lose precision; the test is not proving anything")
	}
}
