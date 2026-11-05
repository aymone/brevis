package sdk

import (
	"errors"
	"strings"
	"testing"
)

// The messages are asserted as strings on purpose.
//
// FormatError used to carry a Format field that was interpolated into the
// message and never filled at any of the four sites that build the error, so
// every format error read "format  from ..." with a hole in it -- and no test
// noticed, because no test looked at the message. These do.
func TestFormatErrorMessage(t *testing.T) {
	cases := []struct {
		name  string
		err   *FormatError
		wants string
	}{
		{
			name:  "without record",
			err:   &FormatError{URL: "https://api.example.com/v1", Line: -1, Cause: errors.New("not a JSON object")},
			wants: "format error in https://api.example.com/v1: not a JSON object",
		},
		{
			name:  "with record",
			err:   &FormatError{URL: "https://api.example.com/v1", Line: 7, Cause: errors.New("building source_key: missing lat")},
			wants: "format error in https://api.example.com/v1, record 7: building source_key: missing lat",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.err.Error()
			if got != c.wants {
				t.Errorf("message\n  got:  %q\n  want: %q", got, c.wants)
			}
			// A hole in the message is the defect this test exists to catch:
			// two spaces in a row, or a stray comma.
			if strings.Contains(got, "  ") {
				t.Errorf("the message has a double space, a sign of an empty field interpolated: %q", got)
			}
			if !errors.Is(c.err, ErrFormat) {
				t.Error("FormatError must satisfy errors.Is(err, ErrFormat)")
			}
			if !errors.Is(c.err, c.err.Cause) {
				t.Error("FormatError must unwrap to the cause")
			}
		})
	}
}

func TestSourceErrorMessageHasNoHole(t *testing.T) {
	e := &SourceError{URL: "https://api.example.com/v1", Attempts: 3, Cause: errors.New("connection reset")}
	got := e.Error()
	if strings.Contains(got, "  ") {
		t.Errorf("double space in SourceError: %q", got)
	}
	if !strings.Contains(got, "3 attempt") {
		t.Errorf("the message must say how many attempts there were: %q", got)
	}
}
