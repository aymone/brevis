package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// THE SERVER SPEAKS CODES, NEVER SENTENCES (apierror.go). A failure answered with prose would reach a reader in
// English whatever language the screen is in, and no test of a handler can see it until somebody writes one.
// This reads the sources instead: no `"error": "<words>"` literal, and no string literal handed to fail().
func TestNoAPIFailureCarriesASentence(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	files = append(files, "../auth/auth.go", "../../web/spa/spa.go")
	// A literal with words in it; a quoted code joined to a constant (`{"error":"` + Code + `"}`) is not one.
	sentence := regexp.MustCompile("\"error\":\\s*\"[^\"`]*[ A-Z.,;][^\"`]*\"")
	jsonSentence := regexp.MustCompile("`[^`]*\"error\":\\s*\"[^\"`]*[ A-Z.,;][^\"`]*\"")
	literal := regexp.MustCompile(`\bfail\(w, [^,]+, "`)
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for i, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if sentence.MatchString(line) || jsonSentence.MatchString(line) || literal.MatchString(line) {
				t.Errorf("%s:%d writes a sentence as an API error; use a code from apierror.go: %s", f, i+1, trimmed)
			}
		}
	}
	if checked < 20 {
		t.Fatalf("read %d files; the glob went blind", checked)
	}
}
