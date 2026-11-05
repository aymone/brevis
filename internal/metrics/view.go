package metrics

import (
	"math"
	"sort"
	"time"
)

// Window is the minutes every series in a Snapshot is aligned to: Values[i]
// is the minute starting at Start+i*60 seconds, and the last one is the
// current minute, still filling.
type Window struct {
	Start   int64 `json:"start"`
	Minutes int   `json:"minutes"`
}

// Owner is the state of one named source (a gateway, an API), all replicas
// together. State says what the screens may claim:
//
//	waiting      no delta yet: the first scrape is only a baseline
//	ok           every replica answers
//	partial      some replicas answer, so the numbers are a floor
//	unreachable  none answers; LastSuccess is the last time one did
type Owner struct {
	Name        string     `json:"name"`
	State       string     `json:"state"`
	Replicas    int        `json:"replicas"`
	Reachable   int        `json:"reachable"`
	LastSuccess *time.Time `json:"lastSuccess"`
	// Error is a class (timeout, unreachable, status, unreadable, too_large),
	// never the error's text: that quotes the address.
	Error string `json:"error,omitempty"`
	// Dropped is how many series the last scrape had beyond the caps (too
	// many series, entities or histogram bounds, or a label value too long).
	// Non-zero means the numbers are a floor; it is said on screen.
	Dropped int `json:"dropped,omitempty"`
}

// Latency is a percentile pair estimated from the histogram's buckets, which
// is an upper-bound interpolation and not an exact value.
type Latency struct {
	P50     *float64 `json:"p50Sec"`
	P95     *float64 `json:"p95Sec"`
	Samples float64  `json:"samples"`
	Minutes int      `json:"windowMinutes"`
}

// Entity is one stream or endpoint (or, with empty Owner and Name, all of
// them). A nil in Values is a minute nobody observed; a 0 is a minute that was
// observed and had nothing.
type Entity struct {
	Owner   string                `json:"owner"`
	Name    string                `json:"name"`
	Present bool                  `json:"present"`
	Values  map[string][]*float64 `json:"values"`
	// Covered is how many seconds of each minute were observed.
	Covered []int `json:"covered"`
	// Rate is per minute over the last minute of scrapes; nil when there is
	// no such minute (waiting, or the source is down).
	Rate map[string]*float64 `json:"ratePerMin"`
	// LastHour is what was counted in the last 60 minutes, and ObservedSec
	// how much of that hour was watched: a console that started ten minutes
	// ago says 600.
	LastHour    map[string]float64 `json:"lastHour"`
	ObservedSec int                `json:"observedSec"`
	Gauges      map[string]float64 `json:"gauges,omitempty"`
	P50         []*float64         `json:"latencyP50Sec,omitempty"`
	P95         []*float64         `json:"latencyP95Sec,omitempty"`
	Latency     *Latency           `json:"latency,omitempty"`
	// lat is each minute's merged histogram, kept for the health rule, which
	// needs a percentile over several minutes and not a mean of percentiles.
	lat []map[float64]float64
}

// Snapshot is everything a screen needs from one kind.
type Snapshot struct {
	Configured  bool      `json:"configured"`
	Since       time.Time `json:"since"`
	IntervalSec float64   `json:"intervalSec"`
	Window      Window    `json:"window"`
	Owners      []Owner   `json:"owners"`
	Total       Entity    `json:"total"`
	Entities    []Entity  `json:"entities"`
	// now is the instant the snapshot was read at, for the health rule.
	now time.Time
}

// unconfigured answers for a nil *Collector, so a console without sources and
// one with none yet are the same "not configured".
var unconfigured = New(Config{})

type acc struct {
	owner, name string
	m           [][]float64 // [measure][minute]
	lat         []map[float64]float64
	cov         []float64
	rate        []float64
	rateOK      bool
	gauge       []float64
	present     bool
}

func newAcc(owner, name string, measures, n int) *acc {
	a := &acc{owner: owner, name: name, m: make([][]float64, measures), lat: make([]map[float64]float64, n), cov: make([]float64, n)}
	for i := range a.m {
		a.m[i] = make([]float64, n)
	}
	return a
}

// Snapshot reads the kind's state as of now, over the last `minutes` minutes
// (clamped to the retention). If only is set, just that entity is returned in
// Entities, with the same consolidated Total for the screen's header.
func (c *Collector) Snapshot(k Kind, minutes int, only *[2]string) Snapshot {
	n := min(max(minutes, 1), Retention)
	if c == nil {
		c = unconfigured
	}
	now := c.now()
	snap := Snapshot{Configured: c.Configured(k), Since: c.started, IntervalSec: c.cfg.Interval.Seconds(), Entities: []Entity{}, Owners: []Owner{}, now: now}
	cur := now.Unix() / 60
	snap.Window = Window{Start: (cur - int64(n) + 1) * 60, Minutes: n}
	if !snap.Configured {
		snap.Total = emptyEntity(specs[k], "", "", n)
		if only != nil {
			snap.Entities = append(snap.Entities, emptyEntity(specs[k], only[0], only[1], n))
		}
		return snap
	}
	sp := specs[k]
	nm := len(sp.counters)

	c.mu.RLock()
	defer c.mu.RUnlock()

	byOwner := map[string][]*targetState{}
	var names []string
	for _, t := range c.targets {
		if t.Kind != k {
			continue
		}
		if byOwner[t.Owner] == nil {
			names = append(names, t.Owner)
		}
		byOwner[t.Owner] = append(byOwner[t.Owner], t)
	}
	sort.Strings(names)

	total := newAcc("", "", nm, n)
	ents := map[[2]string]*acc{}
	fresh := 3*c.cfg.Interval + c.client.Timeout
	for _, owner := range names {
		ts := byOwner[owner]
		snap.Owners = append(snap.Owners, ownerState(owner, ts, now, fresh))
		// Coverage of an owner is its best replica per minute: replicas watch
		// the same wall clock, so seconds do not add, events do.
		cov := make([]float64, n)
		var dt float64
		cut := now.Add(-recentWindow)
		for _, t := range ts {
			for i := 0; i < n; i++ {
				m := cur - int64(n-1-i)
				if cc := t.cov; cc != nil && cc[m%Retention].minute == m {
					cov[i] = max(cov[i], cc[m%Retention].secs)
				}
			}
			var d float64
			for _, p := range t.recent {
				if !p.at.Before(cut) {
					d += p.dt
				}
			}
			dt = max(dt, d)
		}
		for i := range cov {
			total.cov[i] = max(total.cov[i], cov[i])
		}
		for _, t := range ts {
			up := !t.lastOK.IsZero() && now.Sub(t.lastOK) <= fresh
			for name, e := range t.ents {
				key := [2]string{owner, name}
				a := ents[key]
				if a == nil {
					a = newAcc(owner, name, nm, n)
					a.cov, a.gauge, a.present = cov, make([]float64, len(sp.gauges)), true
					ents[key] = a
				}
				for i := 0; i < n; i++ {
					m := cur - int64(n-1-i)
					b := &e.ring[m%Retention]
					if b.minute != m || b.v == nil {
						continue
					}
					for j, v := range b.v {
						a.m[j][i] += v
					}
					for k, v := range b.lat {
						if a.lat[i] == nil {
							a.lat[i] = map[float64]float64{}
						}
						a.lat[i][e.les[k]] += v
					}
				}
				if up {
					for j, g := range e.gauge {
						a.gauge[j] += g
					}
				}
			}
			if dt > 0 {
				for _, p := range t.recent {
					if p.at.Before(cut) {
						continue
					}
					for name, d := range p.d {
						a := ents[[2]string{owner, name}]
						if a.rate == nil {
							a.rate = make([]float64, nm)
						}
						for j, v := range d {
							a.rate[j] += v / dt * 60
						}
						a.rateOK = true
					}
				}
			}
		}
		// A stream that was scraped for the whole window with no delta has a
		// rate of zero, not an unknown one.
		if dt > 0 {
			for key, a := range ents {
				if key[0] == owner && a.rate == nil {
					a.rate, a.rateOK = make([]float64, nm), true
				}
			}
		}
	}

	keys := make([][2]string, 0, len(ents))
	for key := range ents {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+"\x00"+keys[i][1] < keys[j][0]+"\x00"+keys[j][1] })
	var totalGauge = make([]float64, len(sp.gauges))
	total.rate = make([]float64, nm)
	for _, key := range keys {
		a := ents[key]
		for j := range a.m {
			for i, v := range a.m[j] {
				total.m[j][i] += v
			}
		}
		for i, l := range a.lat {
			for le, v := range l {
				if total.lat[i] == nil {
					total.lat[i] = map[float64]float64{}
				}
				total.lat[i][le] += v
			}
		}
		if a.rateOK {
			total.rateOK = true
			for j, v := range a.rate {
				total.rate[j] += v
			}
		}
		for j, g := range a.gauge {
			totalGauge[j] += g
		}
		if only == nil || *only == key {
			snap.Entities = append(snap.Entities, a.view(sp, n))
		}
	}
	total.gauge, total.present = totalGauge, len(keys) > 0
	snap.Total = total.view(sp, n)
	if only != nil && len(snap.Entities) == 0 {
		// Known to the console, never seen by the source: an endpoint nobody
		// called yet, or a name that does not match. Present=false says so.
		snap.Entities = append(snap.Entities, emptyEntity(sp, only[0], only[1], n))
	}
	return snap
}

func emptyEntity(sp *spec, owner, name string, n int) Entity {
	if sp == nil {
		sp = &spec{}
	}
	a := newAcc(owner, name, len(sp.counters), n)
	return a.view(sp, n)
}

// view turns an accumulator into JSON-shaped data.
func (a *acc) view(sp *spec, n int) Entity {
	e := Entity{Owner: a.owner, Name: a.name, Present: a.present, Values: map[string][]*float64{}, Covered: make([]int, n),
		Rate: map[string]*float64{}, LastHour: map[string]float64{}}
	e.lat = a.lat
	hour := min(60, n)
	for i := 0; i < n; i++ {
		e.Covered[i] = int(math.Round(a.cov[i]))
		if i >= n-hour {
			e.ObservedSec += e.Covered[i]
		}
	}
	for j, name := range sp.counters {
		vals := make([]*float64, n)
		for i := 0; i < n; i++ {
			if a.cov[i] > 0 {
				v := a.m[j][i]
				vals[i] = &v
				if i >= n-hour {
					e.LastHour[name] += v
				}
			}
		}
		e.Values[name] = vals
		if a.rateOK {
			r := a.rate[j]
			e.Rate[name] = &r
		} else {
			e.Rate[name] = nil
		}
	}
	if len(sp.gauges) > 0 && a.present {
		e.Gauges = map[string]float64{}
		for j, name := range sp.gauges {
			e.Gauges[name] = a.gauge[j]
		}
	}
	{
		e.P50, e.P95 = make([]*float64, n), make([]*float64, n)
		for i, l := range a.lat {
			if q, ok := quantile(l, 0.5); ok {
				e.P50[i] = &q
				q95, _ := quantile(l, 0.95)
				e.P95[i] = &q95
			}
		}
		// "Now" is the last five minutes, merged: a percentile of one
		// partial minute is a handful of requests.
		w := min(5, n)
		merged := map[float64]float64{}
		for _, l := range a.lat[n-w:] {
			for le, v := range l {
				merged[le] += v
			}
		}
		e.Latency = &Latency{Minutes: w, Samples: merged[math.Inf(1)]}
		if q, ok := quantile(merged, 0.5); ok {
			q95, _ := quantile(merged, 0.95)
			e.Latency.P50, e.Latency.P95 = &q, &q95
		}
	}
	return e
}

// quantile is Prometheus' histogram_quantile over cumulative bucket increases:
// the rank is found in the first bucket that holds it and interpolated inside
// it. A rank in the +Inf bucket answers the highest finite bound, because
// nothing is known above it.
func quantile(b map[float64]float64, q float64) (float64, bool) {
	total := b[math.Inf(1)]
	if total <= 0 {
		return 0, false
	}
	les := make([]float64, 0, len(b))
	for le := range b {
		les = append(les, le)
	}
	sort.Float64s(les)
	rank := q * total
	prevLE, prevCount := 0.0, 0.0
	for _, le := range les {
		count := b[le]
		if count >= rank {
			if math.IsInf(le, 1) {
				return prevLE, true
			}
			if count == prevCount {
				return le, true
			}
			return prevLE + (le-prevLE)*(rank-prevCount)/(count-prevCount), true
		}
		prevLE, prevCount = le, count
	}
	return prevLE, true
}

func ownerState(name string, ts []*targetState, now time.Time, fresh time.Duration) Owner {
	o := Owner{Name: name, Replicas: len(ts)}
	var diffs int
	for _, t := range ts {
		if !t.lastOK.IsZero() {
			ok := t.lastOK
			if o.LastSuccess == nil || ok.After(*o.LastSuccess) {
				o.LastSuccess = &ok
			}
		}
		if t.lastErr == "" && !t.lastOK.IsZero() && now.Sub(t.lastOK) <= fresh {
			o.Reachable++
		} else if t.lastErr != "" {
			o.Error = t.lastErr
		}
		diffs += t.diffs
		o.Dropped += t.dropped
	}
	switch {
	case o.Reachable == o.Replicas && diffs == 0:
		o.State = "waiting"
	case o.Reachable == o.Replicas:
		o.State = "ok"
	case o.Reachable > 0:
		o.State = "partial"
	case o.LastSuccess == nil && o.Error == "":
		o.State = "waiting" // nothing attempted yet
	default:
		o.State = "unreachable"
	}
	return o
}
