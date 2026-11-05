package jsontext

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestAppendJSONStringAgreesWithTheEncoder compares byte for byte against
// encoding/json with HTML escaping off.
//
// The claim is not "mine is right": it is "it is identical to the stdlib's".
// Writing
// JSON by hand is how you produce a file the server reads differently, and
// here is worse -- the same rule composes a KEY.
func TestAppendJSONStringAgreesWithTheEncoder(t *testing.T) {
	cases := []struct {
		s            string
		onlySemantic bool
	}{
		{"", false}, {"plain", false}, {"with space", false},
		{`quotes "in the middle"`, false},
		{`backslash \ and \\ double`, false},
		{"break\nof\rline\tand tab", false},
		{"control \x00\x01\x1f in the middle", false},
		{"café and emoji 🎉", false},
		{"html < > & no escape", false},
		{string([]byte{0xff, 0xfe}), true},
		{"café " + string([]byte{0xff}) + " mixed", true},
	}

	for _, c := range cases {
		name := strings.Map(func(r rune) rune {
			if r < 0x20 {
				return '_'
			}
			return r
		}, c.s)
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(c.s); err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSuffix(buf.String(), "\n")

			got := string(AppendJSONString(nil, c.s))
			if got == want {
				return
			}
			if !c.onlySemantic {
				t.Fatalf("%q:\n  mine     %s\n  Encoder  %s", c.s, got, want)
			}

			// Invalid UTF-8: 1.25's encoding/json escapes the U+FFFD and
			// 1.27 writes the bytes. Both forms are the same code point, and
			// the equality that counts there is of the value.
			var mine, dele string
			if err := json.Unmarshal([]byte(got), &mine); err != nil {
				t.Fatalf("my output is not even valid JSON: %s", got)
			}
			if err := json.Unmarshal([]byte(want), &dele); err != nil {
				t.Fatalf("the encoder's output does not decode: %s", want)
			}
			if mine != dele {
				t.Errorf("%q decodes differently:\n  mine    %q\n  Encoder %q", c.s, mine, dele)
			}
		})
	}
}

// TestAppendJSONStringAppendsToTheDestination: it takes the buffer and returns the
// buffer, so it does not allocate one per string on a load of millions.
func TestAppendJSONStringAppendsToTheDestination(t *testing.T) {
	dst := []byte("before:")
	got := string(AppendJSONString(dst, "x"))
	if got != `before:"x"` {
		t.Errorf("= %q", got)
	}
}
