package events

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// Detector is the cheap question "has this topic's state changed?": a string
// that differs whenever what the screen shows may differ. Equal strings mean
// nothing to refetch.
type Detector interface {
	Fingerprint(ctx context.Context) (string, error)
}

// DetectorFunc adapts a function to Detector.
type DetectorFunc func(ctx context.Context) (string, error)

func (f DetectorFunc) Fingerprint(ctx context.Context) (string, error) { return f(ctx) }

// Factory builds the detector for a (validated) topic.
type Factory func(topic string) Detector

var (
	// ErrFull is the 503: the process already serves its limit of connections.
	ErrFull = errors.New("too many live connections")
	// ErrClosed means the hub is shutting down.
	ErrClosed = errors.New("hub closed")
)

// Options tunes a Hub. Zero values are the production defaults.
type Options struct {
	Interval time.Duration // detector period; default 1s
	MaxSubs  int           // concurrent subscriptions; default 500
	// MaxDynamicDetectors bounds the NAMED topics watched at once (run:<id>,
	// workflow:<slug>); default 96. Each is a goroutine and a database query per
	// tick, and the client names them: without this, 500 connections of 20
	// distinct names each would be ten thousand queries a second, and the cap on
	// subscriptions would not have been what stopped it. The fixed topics (six,
	// shared by everyone) are NOT counted: they must always be startable, or one
	// client filling the named slots would keep every other screen from updating.
	MaxDynamicDetectors int
	Log                 *slog.Logger
	// After is the clock: a channel that fires after d, and a stop func.
	// Injected so tests drive the detectors without sleeping.
	After func(d time.Duration) (<-chan time.Time, func())
	// Jitter returns a multiplier near 1 applied to each wait; default +-10%.
	// Without it, detectors started together would all hit the database in
	// the same millisecond every second.
	Jitter func() float64
}

// subBuffer is one subscriber's queue. Coalescing keeps at most one pending
// entry per topic, so overflowing it means the client sits on more unread
// changes than this: it is dropped (and reconnects, refetching everything)
// rather than waited for.
const subBuffer = 8

// Hub fans detector results out to subscribers.
type Hub struct {
	opt     Options
	factory Factory
	ctx     context.Context

	mu     sync.Mutex
	topics map[string]*topic
	subs   map[*Sub]struct{}
	closed bool
}

type topic struct {
	subs   map[*Sub]struct{}
	cancel context.CancelFunc
	ready  chan struct{} // closed once the baseline attempt is done
	done   chan struct{} // closed when the goroutine has exited
}

// NewHub starts a hub that closes every subscription when ctx ends: that is
// how the server's graceful shutdown ends the streams (http.Server.Shutdown
// waits for handlers, and SSE handlers never return by themselves).
func NewHub(ctx context.Context, f Factory, opt Options) *Hub {
	if opt.Interval <= 0 {
		opt.Interval = time.Second
	}
	if opt.MaxSubs <= 0 {
		opt.MaxSubs = 500
	}
	if opt.MaxDynamicDetectors <= 0 {
		opt.MaxDynamicDetectors = 96
	}
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	if opt.After == nil {
		opt.After = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTimer(d)
			return t.C, func() { t.Stop() }
		}
	}
	if opt.Jitter == nil {
		// #nosec G404 -- spreads the detectors' ticks (+-10%) so they do not all hit
		// the database in the same instant; it is timing, not a secret.
		opt.Jitter = func() float64 { return 0.9 + 0.2*rand.Float64() }
	}
	h := &Hub{opt: opt, factory: f, ctx: ctx, topics: map[string]*topic{}, subs: map[*Sub]struct{}{}}
	context.AfterFunc(ctx, h.shutdown)
	return h
}

// Sub is one client's subscription. C yields changed topic names and is
// closed when the subscription ends (Close, drop, or shutdown).
type Sub struct {
	C      <-chan string
	c      chan string
	h      *Hub
	topics []string
	queued map[string]bool // guarded by h.mu
	done   bool            // guarded by h.mu
}

// Subscribe attaches to the topics, starting a detector for each one nobody
// watches yet. It returns after every detector has taken its baseline, so a
// change that lands right after "ready" is seen as a change, not swallowed
// into the baseline. Topics must already be validated (ParseTopics).
func (h *Hub) Subscribe(ctx context.Context, topics []string) (*Sub, error) {
	c := make(chan string, subBuffer)
	s := &Sub{C: c, c: c, h: h, topics: topics, queued: map[string]bool{}}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrClosed
	}
	if len(h.subs) >= h.opt.MaxSubs {
		h.mu.Unlock()
		return nil, ErrFull
	}
	// Count the NAMED detectors this subscription would START (named topics
	// nobody watches yet) and refuse it whole if they do not fit: counted before
	// anything is registered, under the lock, so a refusal leaves no trace.
	live, fresh := 0, 0
	for name := range h.topics {
		if !fixedTopic(name) {
			live++
		}
	}
	for _, name := range topics {
		if h.topics[name] == nil && !fixedTopic(name) {
			fresh++
		}
	}
	if live+fresh > h.opt.MaxDynamicDetectors {
		h.mu.Unlock()
		return nil, ErrFull
	}
	h.subs[s] = struct{}{}
	var waits []chan struct{}
	for _, name := range topics {
		t := h.topics[name]
		if t == nil {
			t = h.start(name)
		}
		t.subs[s] = struct{}{}
		waits = append(waits, t.ready)
	}
	h.mu.Unlock()

	// Bounded: a slow database must not hold the connection open without
	// "ready"; past the bound the baseline race is the lesser evil.
	limit := time.NewTimer(3 * time.Second)
	defer limit.Stop()
	for _, w := range waits {
		select {
		case <-w:
		case <-limit.C:
		case <-ctx.Done():
			s.Close()
			return nil, ctx.Err()
		}
	}
	return s, nil
}

// start launches the detector goroutine. Caller holds h.mu.
func (h *Hub) start(name string) *topic {
	ctx, cancel := context.WithCancel(h.ctx)
	t := &topic{subs: map[*Sub]struct{}{}, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
	h.topics[name] = t
	go h.run(ctx, name, t, h.factory(name))
	return t
}

func (h *Hub) run(ctx context.Context, name string, t *topic, d Detector) {
	defer close(t.done)
	var last string
	var have bool
	probe := func() {
		fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		fp, err := d.Fingerprint(fctx)
		if err != nil {
			// Keep the old value: a blip must neither wake every client nor
			// reset the baseline.
			h.opt.Log.Debug("fingerprint failed", "topic", name, "error", err)
			return
		}
		if have && fp != last {
			h.broadcast(name, t)
		}
		last, have = fp, true
	}
	probe() // the baseline: never broadcast
	close(t.ready)
	for {
		tick, stop := h.opt.After(time.Duration(float64(h.opt.Interval) * h.opt.Jitter()))
		select {
		case <-ctx.Done():
			stop()
			return
		case <-tick:
			probe()
		}
	}
}

func (h *Hub) broadcast(name string, t *topic) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.topics[name] != t {
		return
	}
	for s := range t.subs {
		if s.queued[name] {
			continue // coalesce: a change is already waiting for this client
		}
		select {
		case s.c <- name:
			s.queued[name] = true
		default:
			h.drop(s)
		}
	}
}

// Ack tells the hub the client consumed a change, so the next one for that
// topic may be queued again.
func (s *Sub) Ack(topic string) {
	s.h.mu.Lock()
	delete(s.queued, topic)
	s.h.mu.Unlock()
}

// Close ends the subscription and frees its detectors if it was the last
// watcher. Safe to call more than once.
func (s *Sub) Close() {
	s.h.mu.Lock()
	s.h.drop(s)
	s.h.mu.Unlock()
}

// drop removes s and closes its channel. Caller holds h.mu.
func (h *Hub) drop(s *Sub) {
	if s.done {
		return
	}
	s.done = true
	close(s.c)
	delete(h.subs, s)
	for _, name := range s.topics {
		t := h.topics[name]
		if t == nil {
			continue
		}
		delete(t.subs, s)
		if len(t.subs) == 0 {
			t.cancel()
			delete(h.topics, name)
		}
	}
}

func (h *Hub) shutdown() {
	h.mu.Lock()
	h.closed = true
	for s := range h.subs {
		h.drop(s)
	}
	h.mu.Unlock()
}

// Stats reports live subscriptions and detectors, for tests and diagnostics.
func (h *Hub) Stats() (subs, detectors int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs), len(h.topics)
}
