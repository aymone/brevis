package api_test

import (
	"strings"
	"testing"
)

// shellOf is the shell alone -- the sidebar, the top bar and the dialogs
// every page carries -- without the page's own content, which its screen's
// module translates.
func shellOf(body string) string {
	main, end := strings.Index(body, "<main"), strings.Index(body, "</main>")
	if main < 0 || end < 0 {
		return body
	}
	return body[:main] + body[end:]
}

// AND NO ENGLISH IS LEFT IN IT. Every sentence the shell said in English
// before this module, looked for in the Portuguese shell: one that survives
// is a string somebody forgot to move to the catalog.
func TestNoEnglishIsLeftInTheTranslatedShell(t *testing.T) {
	body := shellOf(pageIn(t, dataUI(catalogFake{}).WithEndpoints(endpointsFake{}), "/ingest/loads", "pt-BR", "", false))
	for _, en := range []string{
		">Overview<", ">Runs<", ">Streams<", ">Loads<", `aria-label="Monitor"`, `aria-label="Orchestrate"`, `aria-label="Ingest"`,
		">Projects<", "Search or jump to", "Documentation", ">Account<", "Close navigation", "Open navigation",
		"Switch to the", `aria-label="Keyboard shortcuts"`, "Go to ", "This list", "None of these fire",
		"Search the console", "Search workflows", " navigate<", " open<", "in a new tab", "Every project and",
		"The health of everything",
	} {
		if strings.Contains(body, en) {
			t.Errorf("the Portuguese shell still says %q", en)
		}
	}
}
