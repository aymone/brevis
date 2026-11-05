package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// THE SERVER HAS NO LANGUAGE. Outside this package it may name a sentence (M, MN, Cat, Msg and the typed values
// Num, Dur and At) for the app to say, and it may negotiate and remember WHICH language the reader asked for, and
// hand the catalogs over. It may not render anything: there is no T, N, Number, Date or Elapsed any more, and
// this keeps it that way when somebody writes one back. Negotiating stays in the two files that serve the session.
func TestTheServerOnlyNamesSentencesAndNegotiatesTheLanguage(t *testing.T) {
	allowed := map[string]bool{
		"M": true, "MN": true, "Msg": true, "Cat": true, "Num": true, "Dur": true, "At": true,
		"Bundle": true, "BundleJSON": true, "Negotiate": true, "Remember": true, "Cookie": true, "Langs": true, "Names": true, "Codes": true,
	}
	use := regexp.MustCompile(`\bi18n\.([A-Za-z]+)`)
	checked := 0
	for _, root := range []string{"../../internal", "../../cmd", "../pages", "../components"} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			for i, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, m := range use.FindAllStringSubmatch(line, -1) {
					if !allowed[m[1]] {
						t.Errorf("%s:%d uses i18n.%s: the server has no language; send a Msg (i18n.M) or data", p, i+1, m[1])
					}
					if (m[1] == "Negotiate" || m[1] == "Remember") && !strings.HasSuffix(p, "session.go") && !strings.HasSuffix(p, "ui.go") {
						t.Errorf("%s:%d negotiates the language outside the session endpoints", p, i+1)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 50 {
		t.Fatalf("read %d files; the walk went blind", checked)
	}
}
