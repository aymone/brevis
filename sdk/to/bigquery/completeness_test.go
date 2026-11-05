package bigquery

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// bigquery.Table is an adapter: it translates its fields into the LoadConfig
// the load package consumes. A LoadConfig field it never writes is a
// adjustment the facade's consumer has no way to make -- and nothing breaks,
// nada avisa.
//
// Aconteceu de verdade: a fase 0 parou de repassar Provider e Entity, e toda
// table created since then came out with no cost-attribution labels. No
// test saw it, because no row count changes.
//
// This test reads both files and compares. It is crude on purpose: it
// fails when somebody adds a field to LoadConfig and forgets to wire it up
// here, which is exactly when you want to be bothered.
func TestEveryLoadConfigFieldIsReachable(t *testing.T) {
	fields := fieldsOf(t, "../../internal/core/types.go", "LoadConfig")
	written := writtenBy(t, "bigquery.go")

	// These the adapter does not write, and for stated reasons.
	notApplicable := map[string]string{
		"Format":   "sempre ndjson: é o único formato que o load escreve",
		"Provider": "vem do lote, não do Target -- ver Write",
		"Entity":   "vem do lote, não do Target -- ver Write",
	}

	var missing []string
	for _, c := range fields {
		if written[c] {
			continue
		}
		if _, ok := notApplicable[c]; ok {
			continue
		}
		missing = append(missing, c)
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("LoadConfig has %s, and bigquery.Table neither defines nor declares them: "+
			"inapplicable. Whoever uses the facade cannot tune them, and nothing warns them.",
			strings.Join(missing, ", "))
	}
}

var (
	fieldRe = regexp.MustCompile(`(?m)^\t([A-Z][A-Za-z0-9]*)\s`)
	writeRe = regexp.MustCompile(`(?m)^\t+([A-Z][A-Za-z0-9]*):`)
	attrRe  = regexp.MustCompile(`cfg\.([A-Z][A-Za-z0-9]*)\s*(,|=)`)
)

func fieldsOf(t *testing.T, path, kind string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	s := string(data)
	i := strings.Index(s, "type "+kind+" struct {")
	if i < 0 {
		t.Fatalf("%s not found in %s", kind, path)
	}
	body := s[i:]
	body = body[:strings.Index(body, "\n}")]

	var out []string
	for _, m := range fieldRe.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func writtenBy(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	s := string(data)

	out := map[string]bool{}
	for _, m := range writeRe.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	for _, m := range attrRe.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	return out
}
