package metrics

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func find(ss []Sample, name string, want map[string]string) (Sample, bool) {
next:
	for _, s := range ss {
		if s.Name != name {
			continue
		}
		for k, v := range want {
			if s.Labels[k] != v {
				continue next
			}
		}
		return s, true
	}
	return Sample{}, false
}

func TestParseRealGatewayExposition(t *testing.T) {
	f, err := os.Open("testdata/gateway.prom")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	ss, err := Parse(f, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := find(ss, "brevis_gateway_buffer_records", map[string]string{"stream": "clicks"}); !ok || s.Value != 0 {
		t.Fatalf("buffer gauge missing or wrong: %+v", ss)
	}
}

func TestParseHistogramEscapesAndSpecials(t *testing.T) {
	in := `# TYPE h histogram
h_bucket{stream="a\"b\\c\nd",le="0.5"} 2
h_bucket{stream="a\"b\\c\nd",le="+Inf"} 5
h_sum{stream="a\"b\\c\nd"} 3.5
h_count{stream="a\"b\\c\nd"} 5
# TYPE c counter
c{x="1"} NaN
c{x="2"} +Inf
c{x="3"} 7
`
	ss, err := Parse(strings.NewReader(in), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := find(ss, "h_bucket", map[string]string{"le": "0.5"}); !ok || s.Value != 2 || s.Labels["stream"] != "a\"b\\c\nd" {
		t.Fatalf("bucket/escape: %+v", s)
	}
	if s, ok := find(ss, "h_bucket", map[string]string{"le": "+Inf"}); !ok || s.Value != 5 {
		t.Fatalf("+Inf bucket: %+v", s)
	}
	if _, ok := find(ss, "c", map[string]string{"x": "1"}); ok {
		t.Fatal("NaN kept")
	}
	if _, ok := find(ss, "c", map[string]string{"x": "2"}); ok {
		t.Fatal("Inf kept")
	}
	if s, ok := find(ss, "c", map[string]string{"x": "3"}); !ok || s.Value != 7 {
		t.Fatal("finite counter lost")
	}
}

func TestParseCapsAndGarbage(t *testing.T) {
	big := "x{l=\"" + strings.Repeat("a", 2<<20) + "\"} 1\n"
	if _, err := Parse(strings.NewReader(big), 1<<20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("huge line: %v", err)
	}
	// Under the cap, a long line is still just a line.
	ok := "x{l=\"" + strings.Repeat("a", 200<<10) + "\"} 1\n"
	if ss, err := Parse(strings.NewReader(ok), 1<<20); err != nil || len(ss) != 1 {
		t.Fatalf("long line: %v %d", err, len(ss))
	}
	if _, err := Parse(strings.NewReader("this is { not metrics\n"), 1<<20); err == nil {
		t.Fatal("garbage parsed")
	}
}

// The fixtures are expositions captured from the real binaries (a gateway and a
// brevis-sql-api running natively with traffic), so these tests fail if a
// series is renamed or a label changes: the numbers on screen come from them.
func TestRealExpositionsFeedTheSpecs(t *testing.T) {
	for _, tc := range []struct {
		file string
		kind Kind
		want map[string]bool // entity -> expects a counter or a bucket
	}{
		{"testdata/gateway-traffic.prom", Gateway, map[string]bool{"clicks": true, "orders": true}},
		{"testdata/sqlapi.prom", Endpoints, map[string]bool{"orders": true, "sales": true}},
	} {
		f, err := os.Open(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		ss, err := Parse(f, 8<<20)
		_ = f.Close()
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		seen := map[string]map[class]bool{}
		for _, s := range ss {
			ent, c, _ := specs[tc.kind].classify(s)
			if c == skip {
				continue
			}
			if seen[ent] == nil {
				seen[ent] = map[class]bool{}
			}
			seen[ent][c] = true
		}
		for ent := range tc.want {
			if !seen[ent][counter] || !seen[ent][bucket] {
				t.Errorf("%s: %s lacks counters or histogram buckets: %v", tc.file, ent, seen[ent])
			}
		}
	}
}
