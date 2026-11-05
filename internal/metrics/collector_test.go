package metrics

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// t0 sits on a minute boundary so the arithmetic below is readable.
var t0 = time.Unix(1_700_000_040, 0)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTest(owners ...string) (*Collector, *clock) {
	ck := &clock{t: t0}
	var ts []Target
	for _, o := range owners {
		ts = append(ts, Target{Kind: Gateway, Owner: o, url: "http://x/metrics"})
	}
	return New(Config{Targets: ts, Interval: 5 * time.Second, Now: ck.now}), ck
}

// feed is what scrape does on success: the attempt counts as reachable.
func feed(c *Collector, ck *clock, t *targetState, s []Sample) {
	t.lastTry, t.lastOK, t.lastErr = ck.t, ck.t, ""
	t.ingest(s, ck.t, c.maxGap)
}

func recv(stream string, v float64) Sample {
	return Sample{Name: "brevis_gateway_events_received_total", Labels: map[string]string{"stream": stream, "format": "json"}, Value: v}
}

func val(t *testing.T, c *Collector, want float64, minuteFromEnd int) {
	t.Helper()
	s := c.Snapshot(Gateway, 5, nil)
	got := s.Total.Values["received"]
	i := len(got) - 1 - minuteFromEnd
	if got[i] == nil || *got[i] != want {
		t.Fatalf("minute -%d = %v, want %v (all: %v)", minuteFromEnd, deref(got), want, deref(got))
	}
}

func deref(v []*float64) []any {
	out := make([]any, len(v))
	for i, p := range v {
		if p != nil {
			out[i] = *p
		}
	}
	return out
}

func TestDeltaResetAndNewSeries(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	feed(c, ck, tg, []Sample{recv("a", 100)}) // baseline: counts nothing
	if s := c.Snapshot(Gateway, 5, nil); s.Owners[0].State != "waiting" {
		t.Fatalf("after the baseline: %+v", s.Owners[0])
	}
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, tg, []Sample{recv("a", 130)})
	val(t, c, 30, 0)
	// Counter reset: the new value is the increase since the restart.
	ck.t = t0.Add(10 * time.Second)
	feed(c, ck, tg, []Sample{recv("a", 20)})
	val(t, c, 50, 0)
	// A series that was not there before is new and born at zero.
	ck.t = t0.Add(15 * time.Second)
	feed(c, ck, tg, []Sample{recv("a", 20), recv("b", 7)})
	val(t, c, 57, 0)
	// A process restart is seen by its start time even when the counter has
	// already passed its old value.
	start := func(v float64) Sample { return Sample{Name: "process_start_time_seconds", Value: v} }
	ck.t = t0.Add(20 * time.Second)
	feed(c, ck, tg, []Sample{start(1), recv("a", 25)}) // first sight of a start time: not a restart
	val(t, c, 62, 0)
	ck.t = t0.Add(25 * time.Second)
	feed(c, ck, tg, []Sample{start(2), recv("a", 40)}) // restarted AND already past 25
	val(t, c, 102, 0)

	s := c.Snapshot(Gateway, 5, nil)
	if s.Owners[0].State != "ok" {
		t.Fatalf("state %q", s.Owners[0].State)
	}
	if a := find2(s.Entities, "a"); a == nil || !a.Present {
		t.Fatal("entity a missing")
	}
}

func find2(es []Entity, name string) *Entity {
	for i := range es {
		if es[i].Name == name {
			return &es[i]
		}
	}
	return nil
}

func TestRateIsOverTheLastMinuteOfScrapes(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	feed(c, ck, tg, []Sample{recv("a", 0)})
	for i := 1; i <= 4; i++ {
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		feed(c, ck, tg, []Sample{recv("a", float64(i)*10)}) // 10 events / 5 s
	}
	s := c.Snapshot(Gateway, 5, nil)
	if r := s.Total.Rate["received"]; r == nil || math.Abs(*r-120) > 1e-9 {
		t.Fatalf("rate = %v, want 120/min", r)
	}
	// Silence for longer than the window: the rate is unknown, not 120.
	ck.t = t0.Add(5 * time.Minute)
	s = c.Snapshot(Gateway, 5, nil)
	if s.Total.Rate["received"] != nil {
		t.Fatalf("stale rate kept: %v", *s.Total.Rate["received"])
	}
}

func TestGapIsNotBridged(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	feed(c, ck, tg, []Sample{recv("a", 0)})
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, tg, []Sample{recv("a", 5)})
	// Ten minutes without a scrape, then a counter 6000 higher: that is not
	// a spike in one minute, it is an unobserved interval.
	ck.t = t0.Add(10*time.Minute + 5*time.Second)
	feed(c, ck, tg, []Sample{recv("a", 6005)})
	s := c.Snapshot(Gateway, 12, nil)
	last := s.Total.Values["received"]
	if last[len(last)-1] != nil && *last[len(last)-1] != 0 {
		t.Fatalf("spike invented: %v", deref(last))
	}
	if s.Total.Values["received"][5] != nil { // minutes in the middle: nobody watched
		t.Fatalf("unobserved minute reported as a number: %v", deref(last))
	}
	if *s.Total.Values["received"][len(last)-11] != 5 {
		t.Fatalf("what was seen before the gap changed: %v", deref(last))
	}
}

func TestReplicasAreSummedAndRestartAlone(t *testing.T) {
	c, ck := newTest("gw", "gw")
	a, b := c.targets[0], c.targets[1]
	feed(c, ck, a, []Sample{recv("s", 1000)})
	feed(c, ck, b, []Sample{recv("s", 50)})
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, a, []Sample{recv("s", 1010)})
	feed(c, ck, b, []Sample{recv("s", 5)}) // b restarted
	snap := c.Snapshot(Gateway, 3, nil)
	val(t, c, 15, 0)
	if o := snap.Owners[0]; o.Replicas != 2 || o.Reachable != 2 || o.State != "ok" {
		t.Fatalf("owner: %+v", o)
	}
	// Coverage does not double: two replicas watching the same 5 seconds.
	if got := snap.Total.Covered[len(snap.Total.Covered)-1]; got != 5 {
		t.Fatalf("covered %d s, want 5", got)
	}
	if r := snap.Total.Rate["received"]; r == nil || math.Abs(*r-180) > 1e-9 {
		t.Fatalf("rate %v, want 180/min (15 events in 5 s)", r)
	}
}

func TestMinuteRolloverAndRingWrap(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	feed(c, ck, tg, []Sample{recv("a", 0)})
	v := 0.0
	// 4 hours of one scrape every 5 s with 1 event: the ring (3 h) wraps.
	for i := 1; i <= 4*720; i++ {
		ck.t = t0.Add(time.Duration(i) * 5 * time.Second)
		v++
		feed(c, ck, tg, []Sample{recv("a", v)})
	}
	s := c.Snapshot(Gateway, Retention, nil)
	vals := s.Total.Values["received"]
	if len(vals) != Retention {
		t.Fatal("window size")
	}
	for i, p := range vals[:Retention-1] {
		if p == nil || *p != 12 {
			t.Fatalf("minute %d = %v, want 12", i, p)
		}
	}
	if s.Window.Start != (ck.t.Unix()/60-Retention+1)*60 {
		t.Fatal("window start")
	}
}

func TestQuantile(t *testing.T) {
	inf := math.Inf(1)
	b := map[float64]float64{0.1: 50, 0.5: 90, 1: 100, inf: 100}
	if q, ok := quantile(b, 0.5); !ok || math.Abs(q-0.1) > 1e-9 {
		t.Fatalf("p50 %v", q)
	}
	if q, _ := quantile(b, 0.95); math.Abs(q-0.75) > 1e-9 {
		t.Fatalf("p95 %v", q)
	}
	over := map[float64]float64{1: 10, inf: 100}
	if q, _ := quantile(over, 0.95); q != 1 {
		t.Fatalf("beyond the last bound: %v", q)
	}
	if _, ok := quantile(map[float64]float64{inf: 0}, 0.5); ok {
		t.Fatal("empty histogram has no percentile")
	}
}

func TestEndpointClassification(t *testing.T) {
	sp := specs[Endpoints]
	cases := []struct {
		s    Sample
		want int
		ok   bool
	}{
		{Sample{Name: "brevis_sql_api_requests_total", Labels: map[string]string{"endpoint": "e", "status": "200"}}, 0, true},
		{Sample{Name: "brevis_sql_api_requests_total", Labels: map[string]string{"endpoint": "e", "status": "429"}}, 1, true},
		{Sample{Name: "brevis_sql_api_requests_total", Labels: map[string]string{"endpoint": "e", "status": "400"}}, 2, true},
		{Sample{Name: "brevis_sql_api_requests_total", Labels: map[string]string{"endpoint": "e", "status": "504"}}, 3, true},
		{Sample{Name: "brevis_sql_api_requests_total", Labels: map[string]string{"endpoint": "-", "status": "401"}}, 0, false},
		{Sample{Name: "brevis_sql_api_cache_requests_total", Labels: map[string]string{"endpoint": "e", "result": "hit"}}, 4, true},
		{Sample{Name: "brevis_sql_api_cache_requests_total", Labels: map[string]string{"endpoint": "e", "result": "miss"}}, 5, true},
		{Sample{Name: "brevis_sql_api_cache_requests_total", Labels: map[string]string{"endpoint": "e", "result": "error"}}, 6, true},
		{Sample{Name: "brevis_sql_api_hook_total", Labels: map[string]string{"endpoint": "e", "outcome": "ok"}}, 0, false},
		{Sample{Name: "brevis_sql_api_hook_total", Labels: map[string]string{"endpoint": "e", "outcome": "panic"}}, 7, true},
	}
	for _, tc := range cases {
		_, cl, idx := sp.classify(tc.s)
		if (cl == counter) != tc.ok || (tc.ok && idx != tc.want) {
			t.Errorf("%v: class %v idx %d", tc.s.Labels, cl, idx)
		}
	}
}

func TestParseTargets(t *testing.T) {
	ts, err := ParseTargets(Gateway, "V", "a=http://h1:9090/metrics, a=http://h2:9090/metrics\nb=https://h3/metrics")
	if err != nil || len(ts) != 3 || ts[1].Owner != "a" {
		t.Fatalf("%v %v", ts, err)
	}
	if strings.Contains(ts[0].String(), "h1") {
		t.Fatal("a target prints its address")
	}
	for _, bad := range []string{"nourl", "a=ftp://h", "a=http://u:p@h/metrics", "=http://h", "a=//h"} {
		_, err := ParseTargets(Gateway, "V", bad)
		if err == nil {
			t.Errorf("%q accepted", bad)
		} else if strings.Contains(err.Error(), "u:p") || strings.Contains(err.Error(), "ftp://h") {
			t.Errorf("error quotes the address: %v", err)
		}
	}
	if ts, err := ParseTargets(Gateway, "V", ""); err != nil || len(ts) != 0 {
		t.Fatal("empty list is not an error")
	}
}

func TestScrapeOverHTTP(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("# TYPE brevis_gateway_events_received_total counter\nbrevis_gateway_events_received_total{stream=\"a\",format=\"json\"} " +
				string(rune('0'+n.Add(1))) + "\n"))
		case "/500":
			http.Error(w, "boom", 500)
		case "/redirect":
			http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
		case "/garbage":
			_, _ = w.Write([]byte("{{{"))
		}
	}))
	defer srv.Close()
	ck := &clock{t: t0}
	mk := func(path string) *Collector {
		return New(Config{Targets: []Target{{Kind: Gateway, Owner: "gw", url: srv.URL + path}}, Now: ck.now})
	}
	c := mk("/ok")
	c.scrape(context.Background(), c.targets[0])
	ck.t = t0.Add(5 * time.Second)
	c.scrape(context.Background(), c.targets[0])
	s := c.Snapshot(Gateway, 2, nil)
	if o := s.Owners[0]; o.State != "ok" || o.LastSuccess == nil {
		t.Fatalf("%+v", o)
	}
	if c.Seq(Gateway) != 2 || c.Seq(Endpoints) != 0 {
		t.Fatal("seq")
	}
	for path, why := range map[string]string{"/500": "status", "/redirect": "status", "/garbage": "unreadable"} {
		c := mk(path)
		c.scrape(context.Background(), c.targets[0])
		o := c.Snapshot(Gateway, 2, nil).Owners[0]
		if o.State != "unreachable" || o.Error != why {
			t.Errorf("%s: %+v", path, o)
		}
	}
	srv.Close()
	c = mk("/ok")
	c.scrape(context.Background(), c.targets[0])
	if o := c.Snapshot(Gateway, 2, nil).Owners[0]; o.Error != "unreachable" {
		t.Fatalf("closed server: %+v", o)
	}
}

func TestNotConfigured(t *testing.T) {
	c := New(Config{})
	s := c.Snapshot(Gateway, 30, nil)
	if s.Configured || len(s.Owners) != 0 {
		t.Fatalf("%+v", s)
	}
	var nilC *Collector
	if nilC.Configured(Gateway) || nilC.Seq(Gateway) != 0 {
		t.Fatal("nil collector")
	}
}
