package components

import (
	"os"
	"strings"
	"testing"
)

// THE LICENSE TRAVELS WITH THE PATHS. ISC asks for the notice to be kept
// with the copies, and the copies are in this package.
func TestTheGlyphsCarryTheirLicense(t *testing.T) {
	raw, err := os.ReadFile("LICENSE-lucide")
	if err != nil {
		t.Fatalf("the Lucide license is not beside the glyphs: %v", err)
	}
	if !strings.Contains(string(raw), "ISC License") {
		t.Error("LICENSE-lucide is not the ISC license")
	}
}
