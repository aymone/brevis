package redshift

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk/internal/core"
)

// TestScalarAgreesWithTheEncoder compares byte for byte against encoding/json,
// configured the way EncodeNDJSON configures it.
//
// Writing JSON by hand is how you produce a file the server reads differently --
// and a COPY that accepts a wrong value is worse than one that fails. So the
// claim is not "mine is right": it is "mine is identical to the stdlib's".
//
// The reference is the Encoder with SetEscapeHTML(false), and NOT json.Marshal:
// Marshal escapes <, > and & by default, and the composite path in this file
// does not escape. Comparing against Marshal would make the two paths of the
// SAME file diverge -- which is what this test caught on its first run.
func TestScalarAgreesWithTheEncoder(t *testing.T) {
	// semanticOnly marks the cases where the toolchains THEMSELVES disagree with
	// each other, and where my code therefore cannot equal both.
	//
	// It happened with invalid UTF-8: Go 1.25's encoding/json writes an escaped
	// `\ufffd`, and 1.27's writes U+FFFD's bytes. Both forms are valid JSON and
	// the same code point -- the COPY reads the same thing -- so there the
	// equality that counts is of the VALUE, and not of the bytes.
	//
	// The exception is declared case by case, and not as a "if it differs,
	// decode and compare": a blanket tolerance would hide a genuinely wrong
	// escape.
	cases := []struct {
		value        any
		semanticOnly bool
	}{
		{nil, false}, {true, false}, {false, false},
		{"", false}, {"plain", false}, {"with space", false},
		{`quotes "in the middle"`, false},
		{`backslash \ and \\ double`, false},
		{"break\nof\rline\tand tab", false},
		{"control \x00\x01\x1f in the middle", false},
		{"café and emoji 🎉", false},
		{"slash / and less < greater > and ampersand &", false},
		{string([]byte{0xff, 0xfe}), true},
		{"café " + string([]byte{0xff}) + " mixed", true},
		{int(0), false}, {int(-1), false}, {int(42), false},
		{int32(-2147483648), false}, {int64(9007199254740993), false},
		{uint64(18446744073709551615), false},
		{float64(0), false}, {float64(-1.5), false}, {float64(1e21), false},
		{float64(0.1), false},
	}

	for _, c := range cases {
		name := strings.ReplaceAll(nameOf(c.value), " ", "_")
		t.Run(name, func(t *testing.T) {
			want, err := likeTheEncoder(c.value)
			if err != nil {
				t.Skipf("the stdlib refuses %v", c.value)
			}

			var buf bytes.Buffer
			if !writeScalar(&buf, c.value) {
				t.Fatalf("writeScalar refused %#v, which the encoder accepts", c.value)
			}
			got := buf.String()

			if got == want {
				return
			}
			if !c.semanticOnly {
				t.Fatalf("%#v:\n  ours     %s\n  Encoder  %s", c.value, got, want)
			}

			// Different bytes: the values still have to be the same.
			var ours, theirs any
			if err := json.Unmarshal([]byte(got), &ours); err != nil {
				t.Fatalf("our output is not even valid JSON: %s", got)
			}
			if err := json.Unmarshal([]byte(want), &theirs); err != nil {
				t.Fatalf("the encoder's output does not decode: %s", want)
			}
			if ours != theirs {
				t.Errorf("%#v decodes differently:\n  ours    %#v\n  Encoder %#v",
					c.value, ours, theirs)
			}
		})
	}
}

// TestScalarRefusesWhatItDoesNotKnow: refusing is the contract -- the caller
// falls back to the encoder, which handles it. Accepting and writing it wrong
// would be the defect.
func TestScalarRefusesWhatItDoesNotKnow(t *testing.T) {
	nonScalars := []any{
		map[string]any{"a": 1},
		[]any{1, 2},
		[]byte{1, 2},
		math.NaN(),
		math.Inf(1),
		struct{ A int }{1},
	}
	for _, v := range nonScalars {
		var buf bytes.Buffer
		if writeScalar(&buf, v) {
			t.Errorf("it accepted %#v and wrote %q", v, buf.String())
		}
	}
}

// TestCompositeStillComesOutTheSame: the slow path goes on producing the same
// document, and it is the test that stops the fast path from changing the rest
// of the output.
func TestCompositeStillComesOutTheSame(t *testing.T) {
	got, err := EncodeNDJSON(testEnvelopes(), []string{"text", "document", "list", "number"})
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	line := strings.TrimSpace(string(got))
	if err := json.Unmarshal([]byte(line), &doc); err != nil {
		t.Fatalf("the output is not valid JSON: %q\n%v", line, err)
	}
	if doc["text"] != `with "quotes"` {
		t.Errorf("text = %#v", doc["text"])
	}
	if m, ok := doc["document"].(map[string]any); !ok || m["a"] != float64(1) {
		t.Errorf("document = %#v", doc["document"])
	}
	if l, ok := doc["list"].([]any); !ok || len(l) != 2 {
		t.Errorf("list = %#v", doc["list"])
	}
}

// likeTheEncoder serializes the way EncodeNDJSON serializes composites.
func likeTheEncoder(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func nameOf(v any) string {
	s := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 {
			return '_'
		}
		return r
	}, jsonOrType(v)))
	if len(s) > 30 {
		s = s[:30]
	}
	if s == "" {
		s = "empty"
	}
	return s
}

func jsonOrType(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "invalid"
	}
	return string(b)
}

func testEnvelopes() []core.Envelope {
	return []core.Envelope{{Payload: map[string]any{
		"text":     `with "quotes"`,
		"document": map[string]any{"a": 1},
		"list":     []any{1, "two"},
		"number":   42,
	}}}
}
