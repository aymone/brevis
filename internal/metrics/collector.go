package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// Retention is how many minute buckets are kept per entity: three hours.
	Retention = 180
	// maxBody caps one exposition. A gateway with hundreds of streams is a few
	// hundred KiB; past this it is not a metrics endpoint.
	maxBody = 8 << 20
	// THE CAPS. A target is the operator's, but a misconfigured URL can point
	// at anything that answers text, and a real exporter can explode a label.
	// What a target says never decides how much this process keeps:
	//
	//	maxSeries      samples looked at per scrape; the rest are counted, not kept
	//	maxEntities    streams/endpoints tracked per target
	//	maxLabelValue  bytes of any label value; a longer one drops the series
	//	                (truncating would merge two different names into one)
	//	maxBounds      distinct histogram `le` per entity, first seen win
	//
	// MEMORY BOUND per target, worst case: maxEntities x Retention buckets x
	// (a cell of 24 B + 9 counters x 8 B + maxBounds x 8 B) = 1000 x 180 x ~600 B
	// = ~110 MB, and only when every entity is full of traffic for three hours;
	// a gateway with 20 streams and 12 bounds is ~1 MB. The previous scrape
	// (prev) is at most maxSeries entries. Truncation is surfaced (Owner.Dropped),
	// never silent.
	maxSeries     = 20_000
	maxEntities   = 1_000
	maxLabelValue = 256
	maxBounds     = 64
	// recentWindow is what "now" means: the last minute of scrapes.
	recentWindow = 60 * time.Second
)

// Config is the collector's knobs. Zero values are the defaults.
type Config struct {
	Targets  []Target
	Interval time.Duration // default 5s, clamped to [1s, 60s]
	Log      *slog.Logger
	// Client and Now are for tests.
	Client *http.Client
	Now    func() time.Time
}

// Collector scrapes its targets and keeps the buckets.
type Collector struct {
	cfg     Config
	started time.Time
	client  *http.Client
	now     func() time.Time
	log     *slog.Logger
	maxGap  time.Duration

	mu      sync.RWMutex
	targets []*targetState
	seq     map[Kind]*atomic.Uint64

	// runCtx and wg exist once Run has started: a target that arrives later
	// (a pod that was scaled up) gets its loop started by SetTargets.
	runCtx context.Context
	wg     sync.WaitGroup
}

type bucketCell struct {
	minute int64
	v      []float64
	// lat[i] is the increase of the cumulative bucket les[i] of its entity: a
	// slice and not a map, so 64 bounds x 180 minutes is 92 KB, not a hash
	// table per minute.
	lat []float64
}

type entityState struct {
	// les is the entity's admitted bucket bounds, in first-seen order and at
	// most maxBounds. A bound outside it is ignored for good.
	les     []float64
	ring    []bucketCell
	gauge   []float64
	gaugeAt time.Time
}

type point struct {
	at time.Time
	dt float64
	d  map[string][]float64
}

type covCell struct {
	minute int64
	secs   float64
}

type targetState struct {
	Target
	spec *spec

	// stop ends the target's scrape loop. retired is when the pod stopped being
	// announced: the target keeps its history (a replica scaled down does not
	// take its past minutes with it) but is no longer a replica, so it never
	// counts as a silent one.
	stop    context.CancelFunc
	retired time.Time
	// synthetic marks the stand-in a reader builds from database rows to carry
	// an owner's entities: it is no replica and never counts as one.
	synthetic bool

	// last attempt
	lastTry, lastOK time.Time
	lastErr         string // a class, never the error's text (it may hold the URL)
	diffs           int    // successful scrapes that produced a delta
	dropped         int    // series of the last scrape that a cap kept out

	prev      map[string]seriesVal
	prevAt    time.Time
	prevStart float64

	ents   map[string]*entityState
	cov    []covCell
	recent []point
}

type seriesVal struct {
	ent string
	c   class
	idx int
	le  float64
	v   float64
}

// New builds a collector. With no targets it does nothing and reports itself
// as not configured, which is the state of every installation where no pod
// announced itself.
func New(cfg Config) *Collector {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	cfg.Interval = min(max(cfg.Interval, time.Second), time.Minute)
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	c := &Collector{cfg: cfg, client: cfg.Client, now: cfg.Now, log: cfg.Log, seq: map[Kind]*atomic.Uint64{
		Gateway: new(atomic.Uint64), Endpoints: new(atomic.Uint64)}}
	if c.now == nil {
		c.now = time.Now
	}
	c.started = c.now()
	// A gap longer than this is not bridged: the delta over it would land in
	// one minute as a spike that never happened. The baseline is dropped and
	// the minutes in between stay "not observed".
	c.maxGap = max(3*cfg.Interval, 15*time.Second)
	if c.client == nil {
		c.client = &http.Client{
			Timeout: min(cfg.Interval, 5*time.Second),
			// Keep-alive on a small pool: one scrape every few seconds should
			// not be a new TCP connection each time. No proxy, no redirect:
			// the address is the operator's and is the only one called.
			Transport:     &http.Transport{MaxIdleConnsPerHost: 2, IdleConnTimeout: 90 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	for _, t := range cfg.Targets {
		c.targets = append(c.targets, newTargetState(t))
	}
	return c
}

func newTargetState(t Target) *targetState {
	return &targetState{Target: t, spec: specs[t.Kind], ents: map[string]*entityState{}}
}

// SetTargets replaces the live targets with ts. A target already known (same
// kind, owner and address) keeps its baseline and history; a new one starts
// scraping at once when Run is going; one that is gone is retired: its loop
// stops and it stops counting as a replica, but its past minutes stay for the
// retention window. At most maxTargets are taken.
func (c *Collector) SetTargets(ts []Target) {
	if len(ts) > maxTargets {
		ts = ts[:maxTargets]
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	want := make(map[string]Target, len(ts))
	for _, t := range ts {
		want[t.key()] = t
	}
	changed := map[Kind]bool{}
	kept := c.targets[:0]
	for _, t := range c.targets {
		_, live := want[t.key()]
		switch {
		case live:
			delete(want, t.key())
			if !t.retired.IsZero() {
				// Back again at the same address: a fresh baseline, since the
				// gap would otherwise be bridged as one spike.
				t.retired, t.prev = time.Time{}, nil
				c.start(t)
				changed[t.Kind] = true
			}
		case t.retired.IsZero():
			t.retired = now
			if t.stop != nil {
				t.stop()
			}
			changed[t.Kind] = true
		case now.Sub(t.retired) > Retention*time.Minute:
			continue // its last minute left the retention window
		}
		kept = append(kept, t)
	}
	c.targets = capRetired(kept)
	for _, t := range ts { // in the caller's order, which is stable
		if _, ok := want[t.key()]; !ok {
			continue
		}
		delete(want, t.key())
		ns := newTargetState(t)
		c.targets = append(c.targets, ns)
		c.start(ns)
		changed[t.Kind] = true
	}
	for k := range changed {
		c.seq[k].Add(1) // the console redraws: the set of replicas changed
	}
}

// maxRetired bounds the retired targets kept for their history. A pod that
// restarts in a loop is a new address every time, and each one keeps a full
// state for the retention window: without a bound, churn is memory.
const maxRetired = maxTargets

// capRetired drops the oldest retired targets beyond maxRetired and keeps the
// live ones and the order of the rest.
func capRetired(ts []*targetState) []*targetState {
	var retired []*targetState
	for _, t := range ts {
		if !t.retired.IsZero() {
			retired = append(retired, t)
		}
	}
	if len(retired) <= maxRetired {
		return ts
	}
	sort.Slice(retired, func(i, j int) bool { return retired[i].retired.Before(retired[j].retired) })
	drop := make(map[*targetState]bool, len(retired)-maxRetired)
	for _, t := range retired[:len(retired)-maxRetired] {
		drop[t] = true
	}
	out := ts[:0]
	for _, t := range ts {
		if !drop[t] {
			out = append(out, t)
		}
	}
	return out
}

// Interval is how often each source is read, after the clamp.
func (c *Collector) Interval() time.Duration { return c.cfg.Interval }

// Configured reports whether any live target of the kind exists.
func (c *Collector) Configured(k Kind) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, t := range c.targets {
		if t.Kind == k && t.retired.IsZero() {
			return true
		}
	}
	return false
}

// Seq is the number of scrape attempts that finished for the kind. It is the
// fingerprint of the SSE topic, so a change event is emitted at most once per
// scrape and never when nothing was scraped.
func (c *Collector) Seq(k Kind) uint64 {
	if c == nil {
		return 0
	}
	return c.seq[k].Load()
}

// Run scrapes until ctx ends. Each target has its own loop, started at a
// random offset inside the first interval so a console with ten targets does
// not hit them in the same millisecond, and each wait is +-10%.
func (c *Collector) Run(ctx context.Context) {
	c.mu.Lock()
	c.runCtx = ctx
	for _, t := range c.targets {
		if t.retired.IsZero() {
			c.start(t)
		}
	}
	c.mu.Unlock()
	<-ctx.Done()
	c.wg.Wait()
}

// start launches the target's loop. The caller holds c.mu; it does nothing
// before Run (Run starts what is there) or after ctx ended.
func (c *Collector) start(t *targetState) {
	if c.runCtx == nil || c.runCtx.Err() != nil {
		return
	}
	ctx, stop := context.WithCancel(c.runCtx)
	t.stop = stop
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		// #nosec G404 -- jitter only: spreads the scrapes so targets are not hit together; nothing secret.
		wait := time.Duration(rand.Float64() * float64(c.cfg.Interval))
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			c.scrape(ctx, t)
			// #nosec G404 -- same jitter as above.
			wait = time.Duration(float64(c.cfg.Interval) * (0.9 + 0.2*rand.Float64()))
		}
	}()
}

func (c *Collector) scrape(ctx context.Context, t *targetState) {
	samples, errClass := c.fetch(ctx, t)
	at := c.now()
	c.mu.Lock()
	t.lastTry = at
	if errClass != "" {
		t.lastErr = errClass
	} else {
		t.lastErr, t.lastOK = "", at
		t.ingest(samples, at, c.maxGap)
	}
	c.mu.Unlock()
	c.seq[t.Kind].Add(1)
	if errClass != "" && ctx.Err() == nil {
		// The target by owner name and the class; the URL never.
		c.log.Debug("metrics scrape failed", "target", t.String(), "why", errClass)
	}
}

func (c *Collector) fetch(ctx context.Context, t *targetState) ([]Sample, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return nil, "unreachable"
	}
	req.Header.Set("Accept", "text/plain;version=0.0.4")
	resp, err := c.client.Do(req)
	if err != nil {
		// err is a *url.Error that quotes the URL: reduced to a class here.
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return nil, "timeout"
		}
		return nil, "unreachable"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // keeps the connection reusable
		return nil, "status"
	}
	samples, err := Parse(resp.Body, maxBody)
	switch {
	case errors.Is(err, ErrTooLarge):
		return nil, "too_large"
	case err != nil:
		return nil, "unreadable"
	}
	return samples, ""
}

func isTimeout(err error) bool {
	var te interface{ Timeout() bool }
	return errors.As(err, &te) && te.Timeout()
}

// ingest folds one scrape into the target's state. at must not go backwards.
// The caller holds the collector's lock.
//
// THE RATE MATH, in one place:
//   - a series' increase is cur-prev; cur<prev is a counter reset (the process
//     restarted) and the increase is cur, as Prometheus' increase() does;
//   - a series absent from the previous scrape is new, born at 0: its increase
//     is cur. The first scrape of a target is only a baseline, so nothing is
//     counted for a process that was already running;
//   - process_start_time_seconds changing is a restart even when every counter
//     already passed its old value, so everything is treated as reset;
//   - replicas are separate targets with separate baselines and are summed
//     when read, so one restarting does not disturb the others;
//   - the increase is attributed to the minute of the scrape that saw it, so
//     events can sit up to one interval after the minute they happened in.
func (t *targetState) ingest(samples []Sample, at time.Time, maxGap time.Duration) {
	cur := make(map[string]seriesVal, min(len(samples), maxSeries))
	gauges := map[string][]float64{}
	var start float64
	t.dropped = 0
	for i, s := range samples {
		if i >= maxSeries {
			t.dropped += len(samples) - i
			break
		}
		if s.Name == "process_start_time_seconds" {
			start = s.Value
			continue
		}
		ent, c, idx := t.spec.classify(s)
		if c == skip {
			continue
		}
		if longLabel(s.Labels) {
			t.dropped++
			continue
		}
		e, known := t.ents[ent]
		if !known {
			if len(t.ents) >= maxEntities {
				t.dropped++
				continue
			}
			e = t.entity(ent)
		}
		switch c {
		case gauge:
			if gauges[ent] == nil {
				gauges[ent] = make([]float64, len(t.spec.gauges))
			}
			gauges[ent][idx] += s.Value
		default:
			sv := seriesVal{ent: ent, c: c, idx: idx, v: s.Value}
			if c == bucket {
				le, ok := parseLE(s.Labels["le"])
				if !ok {
					continue
				}
				if sv.idx = e.bound(le); sv.idx < 0 {
					t.dropped++
					continue
				}
				sv.le = le
			}
			cur[rawKey(s)] = sv
		}
	}
	for ent, g := range gauges {
		e := t.ents[ent]
		e.gauge, e.gaugeAt = g, at
	}

	if t.prev != nil && at.After(t.prevAt) && at.Sub(t.prevAt) <= maxGap {
		restarted := start != 0 && t.prevStart != 0 && start != t.prevStart
		dt := at.Sub(t.prevAt).Seconds()
		minute := at.Unix() / 60
		pt := point{at: at, dt: dt, d: map[string][]float64{}}
		for k, c := range cur {
			d := c.v
			if p, ok := t.prev[k]; ok && !restarted && c.v >= p.v {
				d = c.v - p.v
			}
			if d == 0 {
				continue
			}
			b := t.ents[c.ent].slot(minute, len(t.spec.counters))
			if c.c == bucket {
				// idx is the bound's position in the entity's admitted set.
				for len(b.lat) <= c.idx {
					b.lat = append(b.lat, 0)
				}
				b.lat[c.idx] += d
				continue
			}
			b.v[c.idx] += d
			if pt.d[c.ent] == nil {
				pt.d[c.ent] = make([]float64, len(t.spec.counters))
			}
			pt.d[c.ent][c.idx] += d
		}
		t.addCov(minute, dt)
		t.recent = append(t.recent, pt)
		t.diffs++
	}
	// Old points leave on every scrape so the slice stays at ~12 entries.
	cut := at.Add(-recentWindow)
	keep := t.recent[:0]
	for _, p := range t.recent {
		if !p.at.Before(cut) {
			keep = append(keep, p)
		}
	}
	t.recent = keep
	t.prev, t.prevAt, t.prevStart = cur, at, start
}

func (t *targetState) entity(name string) *entityState {
	e := t.ents[name]
	if e == nil {
		e = &entityState{ring: make([]bucketCell, Retention)}
		t.ents[name] = e
	}
	return e
}

// bound is the position of le in the entity's admitted bounds, admitting it
// while there is room; -1 when the set is full and le is not in it.
func (e *entityState) bound(le float64) int {
	for i, b := range e.les {
		if b == le {
			return i
		}
	}
	if len(e.les) >= maxBounds {
		return -1
	}
	e.les = append(e.les, le)
	return len(e.les) - 1
}

func longLabel(ls map[string]string) bool {
	for _, v := range ls {
		if len(v) > maxLabelValue {
			return true
		}
	}
	return false
}

// slot is the minute's cell, recycled when the ring has wrapped onto it.
func (e *entityState) slot(minute int64, n int) *bucketCell {
	b := &e.ring[minute%Retention]
	if b.minute != minute || b.v == nil {
		*b = bucketCell{minute: minute, v: make([]float64, n)}
	}
	return b
}

func (t *targetState) addCov(minute int64, secs float64) {
	if t.cov == nil {
		t.cov = make([]covCell, Retention)
	}
	c := &t.cov[minute%Retention]
	if c.minute != minute {
		*c = covCell{minute: minute}
	}
	c.secs += secs
}

func parseLE(s string) (float64, bool) {
	if s == "+Inf" {
		return math.Inf(1), true
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil && !math.IsNaN(f)
}

// rawKey identifies a series across scrapes: name plus sorted labels.
func rawKey(s Sample) string {
	keys := make([]string, 0, len(s.Labels))
	for k := range s.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(s.Name)
	for _, k := range keys {
		b.WriteByte(0xff)
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(s.Labels[k])
	}
	return b.String()
}
