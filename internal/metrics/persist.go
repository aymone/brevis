package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// THE SPLIT. One process collects (the scheduler, holding an advisory lock):
// it scrapes the pods the annotations name, keeps the baselines in memory, and
// writes the result to Postgres once per scrape round. Every API process only
// reads those rows and rebuilds the same Snapshot the collector would have
// answered, so the health function and the views run unchanged over rows and
// every API pod answers identically.

// SourceRow is one replica as the collector last saw it.
type SourceRow struct {
	Kind        Kind
	Owner       string
	Instance    string
	Started     time.Time
	IntervalSec float64
	LastTry     time.Time // zero: never
	LastOK      time.Time
	LastError   string
	Diffs       int
	Dropped     int
	// Age is how long ago the collector last wrote the row, measured by the
	// database's clock; set on read only.
	Age time.Duration
}

// MinuteRow is one entity's minute, summed over the owner's replicas. Entity
// "" is the owner's own row and carries only Covered. Gauges and Rate are set
// on the newest minute only.
type MinuteRow struct {
	Kind    Kind
	Owner   string
	Entity  string
	Minute  int64
	Counts  []float64
	Latency map[string]float64 // le -> increase of the cumulative bucket
	Covered float64
	Gauges  []float64
	Rate    []float64
	// Age is how long ago the row was written, by the database's clock; read only.
	Age time.Duration
}

// Export is what one collector round writes.
type Export struct {
	Sources []SourceRow
	Minutes []MinuteRow
}

// Rows is what a reader loads for one kind.
type Rows struct {
	Sources []SourceRow
	Minutes []MinuteRow
}

// Store is the database side. The postgres package implements it.
type Store interface {
	// Write upserts the export in one transaction and removes the sources of
	// the kinds it covers that it no longer lists.
	Write(ctx context.Context, e Export) error
	// Load reads the sources and the minutes from `sinceMinute` on.
	Load(ctx context.Context, k Kind, sinceMinute int64) (Rows, error)
	// Fingerprint changes whenever a collector round wrote something for k.
	Fingerprint(ctx context.Context, k Kind) (string, error)
	// Prune deletes minutes older than minute, at most limit rows, and
	// returns how many went.
	Prune(ctx context.Context, beforeMinute int64, limit int) (int, error)
}

func instanceOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return "unknown"
}

// Export is the collector's state as rows: every live replica, and for each
// owner with one the current and the previous minute of every entity (the
// previous one because a scrape is attributed to the minute it finished in,
// and the last round of a minute may land after the last write of it).
func (c *Collector) Export() Export {
	now := c.now()
	cur := now.Unix() / 60
	c.mu.RLock()
	defer c.mu.RUnlock()

	type ok struct {
		k     Kind
		owner string
	}
	owners := map[ok][]*targetState{}
	var keys []ok
	var ex Export
	fresh := 3*c.cfg.Interval + c.client.Timeout
	for _, t := range c.targets {
		k := ok{t.Kind, t.Owner}
		if owners[k] == nil {
			keys = append(keys, k)
		}
		owners[k] = append(owners[k], t)
		if t.retired.IsZero() {
			ex.Sources = append(ex.Sources, SourceRow{Kind: t.Kind, Owner: t.Owner, Instance: instanceOf(t.url),
				Started: c.started, IntervalSec: c.cfg.Interval.Seconds(), LastTry: t.lastTry, LastOK: t.lastOK,
				LastError: t.lastErr, Diffs: t.diffs, Dropped: t.dropped})
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i].k)+"\x00"+keys[i].owner < string(keys[j].k)+"\x00"+keys[j].owner
	})
	cut := now.Add(-recentWindow)
	for _, key := range keys {
		ts := owners[key]
		live := false
		for _, t := range ts {
			live = live || t.retired.IsZero()
		}
		if !live {
			continue // an owner nobody announces any more
		}
		sp := specs[key.k]
		var dt float64
		names := map[string]bool{}
		for _, t := range ts {
			var d float64
			if t.retired.IsZero() {
				for _, p := range t.recent {
					if !p.at.Before(cut) {
						d += p.dt
					}
				}
			}
			dt = max(dt, d)
			for n := range t.ents {
				names[n] = true
			}
		}
		sorted := make([]string, 0, len(names))
		for n := range names {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)
		for m := cur - 1; m <= cur; m++ {
			var cov float64
			for _, t := range ts {
				if t.cov != nil && t.cov[m%Retention].minute == m {
					cov = max(cov, t.cov[m%Retention].secs)
				}
			}
			if cov > 0 {
				ex.Minutes = append(ex.Minutes, MinuteRow{Kind: key.k, Owner: key.owner, Minute: m, Counts: make([]float64, len(sp.counters)), Covered: cov})
			}
			for _, name := range sorted {
				row := MinuteRow{Kind: key.k, Owner: key.owner, Entity: name, Minute: m, Counts: make([]float64, len(sp.counters)), Latency: map[string]float64{}}
				any := false
				for _, t := range ts {
					e := t.ents[name]
					if e == nil {
						continue
					}
					b := &e.ring[m%Retention]
					if b.minute != m || b.v == nil {
						continue
					}
					any = true
					for j, v := range b.v {
						row.Counts[j] += v
					}
					for i, v := range b.lat {
						row.Latency[strconv.FormatFloat(e.les[i], 'g', -1, 64)] += v
					}
				}
				if m == cur {
					row.Gauges, row.Rate = c.currentOf(ts, name, sp, dt, cut, now, fresh)
					any = true
				}
				if any {
					ex.Minutes = append(ex.Minutes, row)
				}
			}
		}
	}
	return ex
}

// currentOf is an entity's gauges (summed over replicas that answered lately)
// and its rate over the last minute of scrapes, as Snapshot computes them.
func (c *Collector) currentOf(ts []*targetState, name string, sp *spec, dt float64, cut, now time.Time, fresh time.Duration) (gauges, rate []float64) {
	for _, t := range ts {
		if !t.retired.IsZero() || t.lastOK.IsZero() || now.Sub(t.lastOK) > fresh {
			continue
		}
		if e := t.ents[name]; e != nil && e.gauge != nil {
			if gauges == nil {
				gauges = make([]float64, len(sp.gauges))
			}
			for j, g := range e.gauge {
				gauges[j] += g
			}
		}
	}
	if dt > 0 {
		rate = make([]float64, len(sp.counters))
		for _, t := range ts {
			if !t.retired.IsZero() {
				continue
			}
			for _, p := range t.recent {
				if p.at.Before(cut) {
					continue
				}
				for j, v := range p.d[name] {
					rate[j] += v / dt * 60
				}
			}
		}
	}
	return gauges, rate
}

// Persist writes the collector's state to the store every `every` and prunes
// what is older than `retention` once a minute, until ctx ends. A failed write
// is logged once per streak and retried: the next round carries the state
// anyway (the rows are absolute, not increments).
func (c *Collector) Persist(ctx context.Context, st Store, every, retention time.Duration, log *slog.Logger) {
	failing := false
	lastPrune := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		if err := st.Write(ctx, c.Export()); err != nil {
			if !failing && ctx.Err() == nil {
				log.Warn("writing the live metrics failed; will retry", "error", err)
			}
			failing = true
		} else {
			failing = false
		}
		if now := time.Now(); now.Sub(lastPrune) >= time.Minute {
			lastPrune = now
			PruneOld(ctx, st, c.now(), retention, log)
		}
	}
}

// PruneBatch is the most rows one DELETE removes and PruneBatches the most
// DELETEs of one pass: lowering the retention on a large backlog is spread over
// passes instead of holding the table in one statement.
const (
	PruneBatch   = 5000
	PruneBatches = 50
)

// PruneOld deletes the minutes older than retention, in bounded batches.
func PruneOld(ctx context.Context, st Store, now time.Time, retention time.Duration, log *slog.Logger) {
	before := now.Add(-retention).Unix() / 60
	for range PruneBatches {
		n, err := st.Prune(ctx, before, PruneBatch)
		if err != nil {
			if ctx.Err() == nil {
				log.Warn("pruning the live metrics failed", "error", err)
			}
			return
		}
		if n < PruneBatch {
			return
		}
	}
}

// Reader answers Snapshots from the store, for the processes that do not
// collect.
type Reader struct {
	Store Store
	Now   func() time.Time
}

func NewReader(st Store) *Reader { return &Reader{Store: st, Now: time.Now} }

// SnapshotReader is what the console's handlers ask. A live *Collector
// satisfies it too.
type SnapshotReader interface {
	Read(ctx context.Context, k Kind, minutes int, only *[2]string) (Snapshot, error)
}

// Read is Snapshot, with the context a collector does not need.
func (c *Collector) Read(_ context.Context, k Kind, minutes int, only *[2]string) (Snapshot, error) {
	return c.Snapshot(k, minutes, only), nil
}

func (r *Reader) Read(ctx context.Context, k Kind, minutes int, only *[2]string) (Snapshot, error) {
	now := r.Now()
	rows, err := r.Store.Load(ctx, k, now.Unix()/60-Retention+1)
	if err != nil {
		return Snapshot{}, err
	}
	return FromRows(rows, now).Snapshot(k, minutes, only), nil
}

// FromRows rebuilds a Collector's view of the world out of rows, so Snapshot
// (and through it the health rule) is the one implementation. A source not
// written for a while is a collector that stopped: it is left out, and with no
// source left the kind is not configured.
func FromRows(r Rows, now time.Time) *Collector {
	interval := 5.0
	var started time.Time
	for _, s := range r.Sources {
		interval = s.IntervalSec
		if started.IsZero() || s.Started.Before(started) {
			started = s.Started
		}
	}
	c := New(Config{Interval: time.Duration(interval * float64(time.Second)), Now: func() time.Time { return now },
		Client: &http.Client{Timeout: 5 * time.Second}})
	if !started.IsZero() {
		c.started = started
	}
	fresh := 3*c.cfg.Interval + c.client.Timeout
	type ok struct {
		k     Kind
		owner string
	}
	carriers := map[ok]*targetState{}
	for _, s := range r.Sources {
		if s.Age > fresh+10*time.Second {
			continue
		}
		t := newTargetState(Target{Kind: s.Kind, Owner: s.Owner})
		t.lastTry, t.lastOK, t.lastErr, t.diffs, t.dropped = s.LastTry, s.LastOK, s.LastError, s.Diffs, s.Dropped
		c.targets = append(c.targets, t)
		k := ok{s.Kind, s.Owner}
		cr := carriers[k]
		if cr == nil {
			cr = newTargetState(Target{Kind: s.Kind, Owner: s.Owner})
			cr.synthetic = true
			carriers[k] = cr
			c.targets = append(c.targets, cr)
		}
		if s.LastOK.After(cr.lastOK) {
			cr.lastOK = s.LastOK
		}
	}
	for _, m := range r.Minutes {
		cr := carriers[ok{m.Kind, m.Owner}]
		sp := specs[m.Kind]
		if cr == nil || sp == nil || m.Minute < now.Unix()/60-Retention+1 || m.Minute > now.Unix()/60+1 {
			continue
		}
		if m.Entity == "" {
			cr.addCov(m.Minute, m.Covered)
			continue
		}
		e := cr.entity(m.Entity)
		b := e.slot(m.Minute, len(sp.counters))
		copy(b.v, m.Counts)
		for le, v := range m.Latency {
			f, good := parseLE(le)
			if !good {
				continue
			}
			i := e.bound(f)
			if i < 0 {
				continue
			}
			for len(b.lat) <= i {
				b.lat = append(b.lat, 0)
			}
			b.lat[i] += v
		}
		if m.Age > fresh {
			continue // the newest row is old: the collector stopped; no rate, no gauge
		}
		if m.Gauges != nil {
			e.gauge, e.gaugeAt = m.Gauges, now
		}
		if m.Rate != nil {
			if len(cr.recent) == 0 {
				cr.recent = []point{{at: now, dt: 60, d: map[string][]float64{}}}
			}
			cr.recent[0].d[m.Entity] = m.Rate
		}
	}
	return c
}
