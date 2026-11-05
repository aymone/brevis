package agent

import (
	"os"
	"testing"
)

// `ps` prints a start time in the machine's locale; under one that puts the day before the month
// (en_NZ, pt_BR on macOS) the agent used to refuse its own pid. The C locale is forced for the call, so
// whatever the environment says, the answer is a time.
func TestProcessStartReadsTheStartTimeInAnyLocale(t *testing.T) {
	for _, locale := range []string{"", "en_NZ.UTF-8", "pt_BR.UTF-8"} {
		t.Setenv("LANG", locale)
		t.Setenv("LC_TIME", locale)
		started, err := processStart(os.Getpid())
		if err != nil || started.IsZero() {
			t.Errorf("LANG=%q: processStart(self) = %v, %v; want the start time", locale, started, err)
		}
	}
}
