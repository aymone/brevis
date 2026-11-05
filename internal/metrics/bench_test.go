package metrics

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// The cost of one scrape, for the numbers in the docs: the real gateway
// exposition, and a gateway with 200 streams (about 4,000 series).
func BenchmarkParseAndIngestReal(b *testing.B) {
	raw, err := os.ReadFile("testdata/gateway-traffic.prom")
	if err != nil {
		b.Fatal(err)
	}
	c, ck := newTest("gw")
	tg := c.targets[0]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ss, err := Parse(strings.NewReader(string(raw)), 8<<20)
		if err != nil {
			b.Fatal(err)
		}
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		feed(c, ck, tg, ss)
	}
}

func BenchmarkParseAndIngest200Streams(b *testing.B) {
	var sb strings.Builder
	for s := 0; s < 200; s++ {
		fmt.Fprintf(&sb, "brevis_gateway_events_received_total{stream=\"s%d\",format=\"json\"} %%d\n", s)
		fmt.Fprintf(&sb, "brevis_gateway_events_rejected_total{stream=\"s%d\",reason=\"identity\"} 1\n", s)
		for _, le := range []string{"0.005", "0.01", "0.05", "0.1", "0.5", "1", "5", "+Inf"} {
			fmt.Fprintf(&sb, "brevis_gateway_delivery_seconds_bucket{stream=\"s%d\",sink=\"files\",le=%q} %%d\n", s, le)
		}
		fmt.Fprintf(&sb, "brevis_gateway_buffer_records{stream=\"s%d\"} 3\n", s)
	}
	tpl := sb.String()
	c, ck := newTest("gw")
	tg := c.targets[0]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		body := strings.ReplaceAll(tpl, "%d", fmt.Sprint(i*10))
		ss, err := Parse(strings.NewReader(body), 8<<20)
		if err != nil {
			b.Fatal(err)
		}
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		feed(c, ck, tg, ss)
	}
}

func BenchmarkSnapshot200Streams(b *testing.B) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	for i := 0; i < 20; i++ {
		var ss []Sample
		for s := 0; s < 200; s++ {
			ss = append(ss, recv(fmt.Sprintf("s%d", s), float64(i*10)))
		}
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		feed(c, ck, tg, ss)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = c.Snapshot(Gateway, 30, nil)
	}
}
