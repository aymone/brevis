package execution

import (
	"strings"
	"testing"
)

func TestTheWindowKeepsEverythingWhenItFits(t *testing.T) {
	var j window
	for _, l := range []string{"first", "second", "third"} {
		j.Write(l)
	}
	if got := j.String(); got != "first\nsecond\nthird\n" {
		t.Errorf("output = %q", got)
	}
}

// The ceiling is what stops a `while true; do echo` from filling Postgres's
// disk.
func TestTheWindowRespectsTheCeiling(t *testing.T) {
	var j window
	line := strings.Repeat("x", 200)
	for i := 0; i < 5000; i++ { // ~1 MB
		j.Write(line)
	}
	if n := len(j.String()); n > LogCeiling+512 {
		t.Errorf("kept %d bytes; the ceiling is %d", n, LogCeiling)
	}
}

// Both ends must survive: the start carries the command and the configuration,
// the end carries the reason for the failure. Keeping only one of them loses
// half the diagnosis.
func TestTheWindowKeepsBothEnds(t *testing.T) {
	var j window
	j.Write("START-OF-OUTPUT")
	filler := strings.Repeat("y", 500)
	for i := 0; i < 3000; i++ {
		j.Write(filler)
	}
	j.Write("ERROR-AT-END")

	got := j.String()
	if !strings.Contains(got, "START-OF-OUTPUT") {
		t.Error("it lost the beginning -- the command that ran disappears")
	}
	if !strings.Contains(got, "ERROR-AT-END") {
		t.Error("lost the end -- the reason for the failure is gone")
	}
}

// Truncating in silence makes the reader conclude the program stopped there.
func TestTheWindowSaysWhatItCut(t *testing.T) {
	var j window
	for i := 0; i < 4000; i++ {
		j.Write(strings.Repeat("z", 300))
	}
	if !strings.Contains(j.String(), "omitted by the") {
		t.Error("it cut without saying so")
	}
}
