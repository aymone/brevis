package api_test

import (
	"regexp"
	"testing"
)

// ON A PHONE THE WORKBENCH STACKS. Three zones side by side need width: on a
// 390px screen the tree took two thirds of it and the editor was a strip.
// Below 960px the zones go one under the other, each with a height of its
// own, the page scrolls instead of the panes, and the grips go -- dragging a
// width there is meaningless. What a grip stored (--rail, --editor) stays in
// the browser for the next wide screen. Asserted in the SERVED sheet.
func TestOnAPhoneTheWorkbenchStacks(t *testing.T) {
	_, ui := browsing(t)
	_, css := get(t, ui, "/assets/app.css")
	var narrow string
	for _, m := range regexp.MustCompile(`@media \(max-width:\s*959\.98px\)\{`).FindAllStringIndex(css, -1) {
		depth, i := 1, m[1]
		for ; i < len(css) && depth > 0; i++ {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		narrow += css[m[1]:i]
	}
	if narrow == "" {
		t.Fatal("the served sheet has no rules below 960px")
	}
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`\.workbench\{[^}]*flex-direction:\s*column`),
		regexp.MustCompile(`\.workbench-grip\{[^}]*display:\s*none`),
		regexp.MustCompile(`\.workbench-rail\{[^}]*width:\s*auto`),
		regexp.MustCompile(`\.workbench-results\{[^}]*flex-direction:\s*column`),
		regexp.MustCompile(`main\[data-fill\]\{[^}]*overflow-y:\s*auto`),
	} {
		if !want.MatchString(narrow) {
			t.Errorf("below 960px the sheet lacks %s", want)
		}
	}
}
