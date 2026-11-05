package pages

import (
	"testing"
)

// A stream's health is never "continuous", "healthy" or "failing" while the
// console holds no traffic: it is unknown, and says whether the destination
// is at least known.
func TestAStreamsHealthIsUnknownAndSaysWhy(t *testing.T) {
	cases := []struct {
		name   string
		stream Stream
		reason string
	}{
		{"a named destination", Stream{Sink: Place{URI: "postgres://warehouse/sales/events", Kind: "postgres"}}, ReasonNotReported},
		{"a destination the manifest could not name", Stream{Sink: Place{Note: "no locator"}}, ReasonDestinationUnnamed},
		{"a stream with only a dead letter", Stream{DeadLetter: &Place{URI: "file:///dl/"}}, ReasonDestinationUnnamed},
	}
	for _, c := range cases {
		if got := c.stream.Health(); got.State != HealthUnknown || got.Reason != c.reason {
			t.Errorf("%s: health = %+v, want unknown/%s", c.name, got, c.reason)
		}
	}
}
