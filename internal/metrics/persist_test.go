package metrics

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

// memStore is the database's upsert semantics in memory.
type memStore struct {
	mu      sync.Mutex
	sources map[string]SourceRow
	minutes map[string]MinuteRow
}

func newMem() *memStore {
	return &memStore{sources: map[string]SourceRow{}, minutes: map[string]MinuteRow{}}
}

func (m *memStore) Write(_ context.Context, e Export) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources = map[string]SourceRow{}
	for _, s := range e.Sources {
		m.sources[string(s.Kind)+s.Owner+s.Instance] = s
	}
	for _, r := range e.Minutes {
		m.minutes[string(r.Kind)+"\x00"+r.Owner+"\x00"+r.Entity+"\x00"+time.Unix(r.Minute*60, 0).String()] = r
	}
	return nil
}

func (m *memStore) Load(_ context.Context, k Kind, since int64) (Rows, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out Rows
	for _, s := range m.sources {
		if s.Kind == k {
			out.Sources = append(out.Sources, s)
		}
	}
	for _, r := range m.minutes {
		if r.Kind == k && r.Minute >= since {
			out.Minutes = append(out.Minutes, r)
		}
	}
	return out, nil
}
func (m *memStore) Fingerprint(context.Context, Kind) (string, error) { return "", nil }
func (m *memStore) Prune(_ context.Context, before int64, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k, r := range m.minutes {
		if r.Minute < before {
			delete(m.minutes, k)
			n++
		}
	}
	return n, nil
}

// Two API processes reading the same rows say the same thing, and what they
// say is what the collector itself would have said.
func TestReadersOverTheSameRowsAnswerTheSame(t *testing.T) {
	c, ck := newTest("gw")
	c.SetTargets([]Target{{Kind: Gateway, Owner: "gw", url: "http://10.0.0.1:9090/metrics"}, {Kind: Gateway, Owner: "gw", url: "http://10.0.0.2:9090/metrics"}})
	st := newMem()
	recvTotal, deliv := 0.0, 0.0
	for i := 0; i < 8; i++ {
		ck.t = t0.Add(time.Duration(i) * time.Minute)
		for step := 0; step < 2; step++ {
			for ti, tg := range c.targets {
				recvTotal += float64(10 * (ti + 1))
				deliv += 1
				feed(c, ck, tg, []Sample{recv("s", recvTotal+float64(i)), {Name: "brevis_gateway_batches_total", Labels: map[string]string{"stream": "s", "sink": "x", "outcome": "delivered"}, Value: deliv},
					{Name: "brevis_gateway_buffer_records", Labels: map[string]string{"stream": "s"}, Value: 3}})
			}
			ck.t = ck.t.Add(5 * time.Second)
			_ = st.Write(context.Background(), c.Export())
		}
	}
	live := c.Snapshot(Gateway, 10, nil)
	rows, _ := st.Load(context.Background(), Gateway, ck.t.Unix()/60-Retention+1)
	a, b := FromRows(rows, ck.t), FromRows(rows, ck.t)
	sa, sb := a.Snapshot(Gateway, 10, nil), b.Snapshot(Gateway, 10, nil)
	if !reflect.DeepEqual(sa.Health("gw", "s"), sb.Health("gw", "s")) || !reflect.DeepEqual(sa.Total, sb.Total) {
		t.Fatal("two readers over the same rows disagree")
	}
	if !sa.Configured || len(sa.Owners) != 1 || sa.Owners[0].Replicas != 2 || sa.Owners[0].Reachable != 2 {
		t.Fatalf("%+v", sa.Owners)
	}
	if !reflect.DeepEqual(live.Total.Values, sa.Total.Values) {
		t.Fatalf("the reader's totals differ from the collector's:\nlive %v\nread %v", deref(live.Total.Values["received"]), deref(sa.Total.Values["received"]))
	}
	if !reflect.DeepEqual(live.Health("gw", "s"), sa.Health("gw", "s")) {
		t.Fatalf("health differs:\nlive %+v\nread %+v", live.Health("gw", "s"), sa.Health("gw", "s"))
	}
	if !reflect.DeepEqual(live.Entities[0].Gauges, sa.Entities[0].Gauges) {
		t.Fatalf("gauges differ: %v vs %v", live.Entities[0].Gauges, sa.Entities[0].Gauges)
	}
}

// A collector that stopped: its rows age out and the kind is not configured.
func TestRowsOfAStoppedCollectorAreNoSource(t *testing.T) {
	rows := Rows{Sources: []SourceRow{{Kind: Gateway, Owner: "gw", Instance: "x", IntervalSec: 5, Age: time.Hour, Started: t0}}}
	if FromRows(rows, t0).Snapshot(Gateway, 5, nil).Configured {
		t.Fatal("a source written an hour ago is configured")
	}
	rows.Sources[0].Age = time.Second
	if !FromRows(rows, t0).Snapshot(Gateway, 5, nil).Configured {
		t.Fatal("a source written a second ago is not configured")
	}
}
