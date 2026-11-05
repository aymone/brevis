package components

import (
	"os"
	"strings"
	"testing"
)

// THE LICENCE TRAVELS WITH THE PATHS. ISC asks for the notice to be kept
// with the copies, and the copies are in this package.
func TestTheGlyphsCarryTheirLicence(t *testing.T) {
	raw, err := os.ReadFile("LICENSE-lucide")
	if err != nil {
		t.Fatalf("the Lucide licence is not beside the glyphs: %v", err)
	}
	if !strings.Contains(string(raw), "ISC License") {
		t.Error("LICENSE-lucide is not the ISC licence")
	}
}
