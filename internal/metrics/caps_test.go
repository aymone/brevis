package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

func delivery(stream string, le float64, v float64) Sample {
	return Sample{Name: "brevis_gateway_delivery_seconds_bucket", Labels: map[string]string{"stream": stream, "le": fmt.Sprint(le)}, Value: v}
}

func TestBoundsPerEntityAreCapped(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	hostile := func(v float64) []Sample {
		var ss []Sample
		for i := 0; i < 10_000; i++ {
			ss = append(ss, delivery("a", float64(i)/1000, v))
		}
		return ss
	}
	feed(c, ck, tg, hostile(1))
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, tg, hostile(2))
	if n := len(tg.ents["a"].les); n != maxBounds {
		t.Fatalf("%d bounds kept, want %d", n, maxBounds)
	}
	// The first 64 win and keep working: their increase is counted.
	b := tg.ents["a"].ring[(t0.Add(5*time.Second).Unix()/60)%Retention]
	if len(b.lat) != maxBounds || b.lat[0] != 1 {
		t.Fatalf("bucket increases: %d %v", len(b.lat), b.lat[:2])
	}
	if o := c.Snapshot(Gateway, 3, nil).Owners[0]; o.Dropped != 10_000-maxBounds {
		t.Fatalf("dropped %d is not surfaced: %+v", o.Dropped, o)
	}
}

func TestSeriesEntitiesAndLabelLengthAreCapped(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	var ss []Sample
	for i := 0; i < 100_000; i++ {
		ss = append(ss, recv(fmt.Sprintf("s%d", i), 1))
	}
	feed(c, ck, tg, ss)
	if len(tg.ents) > maxEntities {
		t.Fatalf("%d entities", len(tg.ents))
	}
	if len(tg.prev) > maxSeries {
		t.Fatalf("%d series kept", len(tg.prev))
	}
	if tg.dropped < 100_000-maxEntities {
		t.Fatalf("dropped %d", tg.dropped)
	}
	c, ck = newTest("gw")
	tg = c.targets[0]
	long := []Sample{recv(strings.Repeat("x", maxLabelValue+1), 1), recv("ok", 1)}
	feed(c, ck, tg, long)
	if _, bad := tg.ents[strings.Repeat("x", maxLabelValue+1)]; bad || tg.dropped != 1 {
		t.Fatalf("long label kept (%v) or not counted (%d)", bad, tg.dropped)
	}
}

// A hostile exposition over HTTP: 30k label sets, then a giant line. The
// scrape comes back inside the target timeout and what is kept stays small.
func TestHostileTargetCostsBoundedMemoryAndTime(t *testing.T) {
	var body strings.Builder
	body.WriteString("# TYPE brevis_gateway_events_received_total counter\n")
	for i := 0; i < 30_000; i++ {
		fmt.Fprintf(&body, "brevis_gateway_events_received_total{stream=\"s%d\",format=\"json\"} %d\n", i, i)
	}
	giant := "x{l=\"" + strings.Repeat("a", 9<<20) + "\"} 1\n"
	mode := "many"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "many" {
			_, _ = w.Write([]byte(body.String()))
		} else {
			_, _ = w.Write([]byte(giant))
		}
	}))
	defer srv.Close()
	ck := &clock{t: t0}
	c := New(Config{Targets: []Target{{Kind: Gateway, Owner: "gw", url: srv.URL}}, Now: ck.now})

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 10; i++ {
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		start := time.Now()
		c.scrape(context.Background(), c.targets[0])
		// The client's per-target timeout is min(interval, 5s).
		if took := time.Since(start); took > 5*time.Second {
			t.Fatalf("scrape %d took %v", i, took)
		}
	}
	mode = "giant"
	c.scrape(context.Background(), c.targets[0])
	if o := c.Snapshot(Gateway, 3, nil).Owners[0]; o.Error != "too_large" {
		t.Fatalf("giant line: %+v", o)
	}
	mode = "many"
	ck.t = t0.Add(time.Minute)
	c.scrape(context.Background(), c.targets[0])
	runtime.GC()
	runtime.ReadMemStats(&after)
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 150<<20 {
		t.Fatalf("heap grew %d MiB", grew>>20)
	}
	if n := len(c.targets[0].ents); n > maxEntities {
		t.Fatalf("%d entities", n)
	}
	if o := c.Snapshot(Gateway, 3, nil).Owners[0]; o.Dropped == 0 {
		t.Fatalf("truncation not surfaced: %+v", o)
	}
}
