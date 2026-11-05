package components

import (
	"os"
	"strings"
	"testing"
)

// THE MARKS ARE OTHER PEOPLE'S WORK, and their licenses travel with them.
func TestTheLogosLicensesAreBesideThem(t *testing.T) {
	for _, f := range []string{"LICENSE-devicon", "LICENSE-simple-icons"} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s is missing: %v", f, err)
			continue
		}
		if !strings.Contains(string(raw), "trademark") {
			t.Errorf("%s does not say the marks belong to their owners", f)
		}
	}
}
