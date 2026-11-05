package components

import (
	"testing"
	"time"
)

func TestTheDurationPicksItsUnit(t *testing.T) {
	casos := []struct {
		d        time.Duration
		expected string
	}{
		{400 * time.Millisecond, "400ms"},
		{2500 * time.Millisecond, "2.5s"},
		{90 * time.Second, "1m 30s"},
		{3*time.Hour + 4*time.Minute, "3h 04m"},
	}
	for _, c := range casos {
		if got := Duration(&c.d); got != c.expected {
			t.Errorf("Duration(%s) = %s, want %s", c.d, got, c.expected)
		}
	}
	if Duration(nil) != "—" {
		t.Error("a missing duration should become a dash, not a zero")
	}
}
