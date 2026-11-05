package metrics

import (
	"math"
	"time"
)

// A stream's health, derived from the series the collector keeps.
//
// THE RULE: honest or nothing. A state is claimed only from numbers that were
// measured, and every way of not having them is "unknown" with the reason:
// no source, a source that stopped answering, a source that never mentioned
// the stream. A stale source never reads as whatever it last said.
//
//	unknown   the console cannot tell (see the Reason* keys)
//	quiet     the source is healthy and nothing happened in the window
//	healthy   events flowed and nothing needed a person
//	degraded  something was refused, retried, buried or slow, below failing
//	failing   most of the window's work failed, or the buffer keeps refusing
//
// HOW A WINDOW BECOMES A STATE. Each minute m gets a RAW state from the
// trailing window of HealthWindow minutes ending at m (the last bucket is the
// minute still filling). Ratios mean nothing over a handful of events, so
// every ratio has a minimum volume, and "failing" also needs the bad evidence
// in MinBadMinutes distinct minutes: one bad minute cannot flip a stream.
//
// HYSTERESIS, with no memory outside the series. The state shown for minute m
// is the WORST raw state among the last RecoverMinutes minutes: it worsens
// the moment the raw state does, and improves only after RecoverMinutes
// consecutive minutes whose raw state was better. Because the rule is a pure
// function of the ring, every request and every console replica agrees, and
// `since` is found by walking the same function backwards.
//
//	raw:     ok  ok  BAD ok  ok  ok  ok  ok
//	shown:   ok  ok  BAD BAD BAD ok  ok  ok      (RecoverMinutes = HealthWindow - 1)
const (
	// HealthWindow is the minutes a state is judged over. Five: long enough
	// that one noisy minute is outweighed by four others, short enough that a
	// sink that died is seen within a flush or two (the default flush is 2
	// minutes), and the same "now" the latency percentiles already use.
	HealthWindow = 5
	// RecoverMinutes is the raw minutes that must ALL be better before the
	// state improves: HealthWindow-1. A stream whose bad minutes come at least
	// once per window (the noisy case, a flapping sink) has clean spells at
	// most HealthWindow-1 minutes long, so a hold that long cannot flap on
	// it; and a stream that really recovered waits about 5 + 3 minutes.
	RecoverMinutes = HealthWindow - 1
	// MinEvents is the volume under which an event ratio is not claimed as
	// "failing": 3 of 4 rejected is an anecdote, 15 of 20 is a pattern. Below
	// it the bad events still make the stream degraded, as counts.
	MinEvents = 20
	// MinBatches is the same for delivery. A batch is minutes of events, so
	// the bar is low: two batches both buried is already a sink that is down.
	MinBatches = 2
	// MinBadMinutes is how many minutes of the window must EACH be failing on
	// their own (a majority of that minute's events or batches bad) for the
	// window to be "failing". Two: a spike, however large, is degraded, and a
	// burst straddling a minute boundary does not count twice, because each of
	// its halves has to be a majority of its own minute.
	MinBadMinutes = 2
	// FailingRatio is a clear majority: half or more of the window's events
	// were rejected or buried, or of its batches buried.
	FailingRatio = 0.5
	// DegradedRejectedRatio is 1% of the offered events refused. A client that
	// sends a bad event in a thousand is normal; one in a hundred is a bug
	// somebody should read about. (Rejected is per event, after decode.)
	DegradedRejectedRatio = 0.01
	// DegradedRetryRatio is a quarter of the delivery attempts being retries.
	// The gateway retries by design (four attempts over about seven seconds),
	// so an occasional retry is the sink blinking, not a problem.
	DegradedRetryRatio = 0.25
	// SaturatedFailingMinutes is "continuously": the buffer refused requests
	// in four of the window's five minutes. One refusal is a burst the client
	// retries; this is a sink that cannot keep up.
	SaturatedFailingMinutes = 4
	// SlowDeliverySec is the p95 of a batch's delivery, retries included,
	// above which the stream is degraded: a full retry ladder is about seven
	// seconds, so thirty means a sink that is slow, not one that blinked.
	SlowDeliverySec = 30.0
	// MinLatencySamples is the batches a p95 needs to mean anything.
	MinLatencySamples = 3
	// StaleScrapes is how many scrape intervals without a good scrape make the
	// source unreachable. Three tolerates two lost scrapes (a slow response,
	// a restart) and still reacts in well under a minute at the default 5s.
	StaleScrapes = 3
)

// The states.
const (
	HealthUnknown  = "unknown"
	HealthQuiet    = "quiet"
	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"
	HealthFailing  = "failing"
)

// The reasons are stable machine keys; the app writes the sentence.
const (
	// Unknown.
	ReasonNotReported         = "not_reported"          // the stream has no name to match the gateway's label by
	ReasonDestinationUnnamed  = "destination_unnamed"   // the manifest could not say where it lands
	ReasonSourceNotConfigured = "source_not_configured" // no metrics address for this stream's gateway
	ReasonSourceUnreachable   = "source_unreachable"    // no good scrape lately
	ReasonWaiting             = "waiting"               // the first scrape is only a baseline
	ReasonNotInSource         = "not_in_source"         // the gateway never reported this stream
	// Quiet.
	ReasonNoTraffic     = "no_traffic"      // since: the last minute that had some
	ReasonNoTrafficSeen = "no_traffic_seen" // none in the whole ring (observedMinutes)
	// Healthy.
	ReasonDelivering = "delivering"
	// Degraded.
	ReasonDeadLettered    = "dead_lettered"
	ReasonRejectedSome    = "rejected_some"
	ReasonSaturated       = "saturated"
	ReasonRetrying        = "retrying"
	ReasonSlowDelivery    = "slow_delivery"
	ReasonReplicasPartial = "replicas_partial"
	// Failing.
	ReasonRejectedMost    = "rejected_most"
	ReasonDeadLetterMost  = "dead_letter_most"
	ReasonDeliveryFailing = "delivery_failing"
	ReasonSaturating      = "saturating"
)

// WindowCounts is what the judged window counted. Delivered, Retried and
// Buried are BATCHES; the rest are events (Saturated: refused requests).
type WindowCounts struct {
	Received   float64 `json:"received"`
	Rejected   float64 `json:"rejected"`
	Dropped    float64 `json:"dropped"`
	DeadLetter float64 `json:"deadLetter"`
	Delivered  float64 `json:"delivered"`
	Retried    float64 `json:"retried"`
	Buried     float64 `json:"buried"`
	Saturated  float64 `json:"saturated"`
}

// Health is a stream's state, the stable key of its reason and the numbers
// that justify it. Everything but State and Reason is optional: a client that
// does not know a field ignores it.
type Health struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	// Since is when the state began, when the ring saw it begin; for quiet,
	// the last minute that had traffic.
	Since         *time.Time `json:"since,omitempty"`
	WindowMinutes int        `json:"windowMinutes,omitempty"`
	// ObservedMinutes is how much the ring has watched, for no_traffic_seen.
	ObservedMinutes int `json:"observedMinutes,omitempty"`
	// Recovering: the raw state is already better, and the shown one is held
	// until RecoverMinutes clean minutes passed.
	Recovering       bool          `json:"recovering,omitempty"`
	Window           *WindowCounts `json:"window,omitempty"`
	RejectedRatio    *float64      `json:"rejectedRatio,omitempty"`
	DeadLetterRatio  *float64      `json:"deadLetterRatio,omitempty"`
	FailedBatchRatio *float64      `json:"failedBatchRatio,omitempty"`
	RetryRatio       *float64      `json:"retryRatio,omitempty"`
	LatencyP95Sec    *float64      `json:"latencyP95Sec,omitempty"`
	// Replicas and Reachable are said only when some replica does not answer.
	Replicas  int `json:"replicas,omitempty"`
	Reachable int `json:"reachable,omitempty"`
}

// Minute is one bucket of a stream, oldest first in Input.Minutes. An
// unobserved minute is a gap (the source was down or the console not yet
// running), which is not the same as a minute with nothing in it.
type Minute struct {
	Observed bool
	WindowCounts
	// Lat is the minute's cumulative histogram increase by upper bound.
	Lat map[float64]float64
}

// Input is everything the rule reads. It holds no clock and no collector so a
// test can state each case in one literal.
type Input struct {
	Now time.Time
	// Source is the owner (gateway) the stream belongs to; nil when no source
	// names it.
	Source   *Owner
	Interval time.Duration
	// Present: the source has reported this stream at least once.
	Present bool
	// Start is the start of Minutes[0]; Minutes[len-1] is the current minute.
	Start   time.Time
	Minutes []Minute
}

var rank = map[string]int{HealthQuiet: 1, HealthHealthy: 1, HealthDegraded: 2, HealthFailing: 3}

func unknown(reason string) Health { return Health{State: HealthUnknown, Reason: reason} }

// Assess is a stream's health. See the top of the file for the rule.
func Assess(in Input) Health {
	o := in.Source
	if o == nil {
		return unknown(ReasonSourceNotConfigured)
	}
	if o.State == "waiting" {
		return unknown(ReasonWaiting)
	}
	// Never report anything from a source that stopped answering, whatever
	// the ring says: the ring is the past, and the past is not the present.
	if o.State == "unreachable" || o.Reachable == 0 || o.LastSuccess == nil ||
		in.Now.Sub(*o.LastSuccess) > StaleScrapes*in.Interval {
		return unknown(ReasonSourceUnreachable)
	}
	if !in.Present {
		return unknown(ReasonNotInSource)
	}

	n := len(in.Minutes)
	raws := make([]*Health, n)
	first := -1
	for i := range raws {
		lo := max(0, i-HealthWindow+1)
		if !anyObserved(in.Minutes[lo : i+1]) {
			continue
		}
		h := rawHealth(in.Minutes[lo : i+1])
		raws[i] = &h
		if first < 0 {
			first = i
		}
	}
	if n == 0 || raws[n-1] == nil {
		return unknown(ReasonWaiting)
	}
	// shown is the state of minute i: the worst raw state of the hold, the
	// latest among equals. ok is false when no minute of the hold was judged
	// (the source was silent for the whole of it).
	shown := func(i int) (h Health, held, ok bool) {
		var worst *Health
		for j := max(first, i-RecoverMinutes+1); j <= i; j++ {
			if raws[j] != nil && (worst == nil || rank[raws[j].State] >= rank[worst.State]) {
				worst = raws[j]
			}
		}
		if worst == nil {
			return Health{}, false, false
		}
		return *worst, worst != raws[i], true
	}
	h, held, _ := shown(n - 1)
	h.Recovering = held && rank[raws[n-1].State] < rank[h.State]

	// Since: walk back while the shown state stays the same. Reaching the first
	// minute the ring could judge means the beginning was not seen, so no claim.
	i := n - 1
	for i > first {
		if prev, _, ok := shown(i - 1); !ok || prev.State != h.State {
			break
		}
		i--
	}
	if i > first {
		t := in.Start.Add(time.Duration(i) * time.Minute)
		h.Since = &t
	}

	if h.State == HealthQuiet {
		h.Since = nil
		last := -1
		for j := n - 1; j >= 0; j-- {
			if in.Minutes[j].Observed && in.Minutes[j].active() {
				last = j
				break
			}
		}
		if last >= 0 {
			t := in.Start.Add(time.Duration(last) * time.Minute)
			h.Since, h.Reason = &t, ReasonNoTraffic
		} else {
			h.Reason = ReasonNoTrafficSeen
			for _, m := range in.Minutes {
				if m.Observed {
					h.ObservedMinutes++
				}
			}
		}
	}

	// Some replicas silent: the numbers are a floor. Bad evidence still counts,
	// but a clean result is not claimed for replicas that were not heard.
	if o.Reachable < o.Replicas {
		h.Replicas, h.Reachable = o.Replicas, o.Reachable
		if rank[h.State] < rank[HealthDegraded] {
			h.State, h.Reason, h.Since, h.ObservedMinutes = HealthDegraded, ReasonReplicasPartial, nil, 0
		}
	}
	return h
}

func anyObserved(ms []Minute) bool {
	for _, m := range ms {
		if m.Observed {
			return true
		}
	}
	return false
}

func (m Minute) active() bool {
	return m.Received+m.Rejected+m.Dropped+m.DeadLetter+m.Delivered+m.Retried+m.Buried+m.Saturated > 0
}

// rawHealth judges one window. Negative numbers (which the collector never
// produces; a reset is handled where counters are read) count as zero.
func rawHealth(ms []Minute) Health {
	var w WindowCounts
	var badRej, badDead, badBur, satMin int
	major := func(bad, all float64) bool { return bad > 0 && bad >= FailingRatio*all }
	lat := map[float64]float64{}
	for _, m := range ms {
		w.Received += max(m.Received, 0)
		w.Rejected += max(m.Rejected, 0)
		w.Dropped += max(m.Dropped, 0)
		w.DeadLetter += max(m.DeadLetter, 0)
		w.Delivered += max(m.Delivered, 0)
		w.Retried += max(m.Retried, 0)
		w.Buried += max(m.Buried, 0)
		w.Saturated += max(m.Saturated, 0)
		badRej += b2i(major(m.Rejected, m.Received+m.Rejected))
		badDead += b2i(major(m.DeadLetter, m.Received))
		badBur += b2i(major(m.Buried, m.Delivered+m.Buried))
		satMin += b2i(m.Saturated > 0)
		for le, v := range m.Lat {
			lat[le] += v
		}
	}
	h := Health{WindowMinutes: HealthWindow, Window: &w}
	if !(Minute{WindowCounts: w}).active() {
		h.State = HealthQuiet
		return h
	}
	ratio := func(num, den float64, min float64) *float64 {
		if den < min || den <= 0 {
			return nil
		}
		r := math.Min(num/den, 1)
		return &r
	}
	offered := w.Received + w.Rejected
	h.RejectedRatio = ratio(w.Rejected, offered, 1)
	h.DeadLetterRatio = ratio(w.DeadLetter, w.Received, MinEvents)
	h.FailedBatchRatio = ratio(w.Buried, w.Delivered+w.Buried, MinBatches)
	h.RetryRatio = ratio(w.Retried, w.Delivered+w.Buried+w.Retried, MinBatches)
	if q, ok := quantile(lat, 0.95); ok && lat[math.Inf(1)] >= MinLatencySamples {
		h.LatencyP95Sec = &q
	}
	atLeast := func(r *float64, v float64) bool { return r != nil && *r >= v }

	failing := func(reason string) Health { h.State, h.Reason = HealthFailing, reason; return h }
	switch {
	case offered >= MinEvents && atLeast(h.RejectedRatio, FailingRatio) && badRej >= MinBadMinutes:
		return failing(ReasonRejectedMost)
	case atLeast(h.DeadLetterRatio, FailingRatio) && badDead >= MinBadMinutes:
		return failing(ReasonDeadLetterMost)
	case atLeast(h.FailedBatchRatio, FailingRatio) && badBur >= MinBadMinutes:
		return failing(ReasonDeliveryFailing)
	case satMin >= SaturatedFailingMinutes:
		return failing(ReasonSaturating)
	}

	degraded := func(reason string) Health { h.State, h.Reason = HealthDegraded, reason; return h }
	switch {
	case w.DeadLetter > 0 || w.Buried > 0:
		return degraded(ReasonDeadLettered)
	case w.Rejected > 0 && atLeast(h.RejectedRatio, DegradedRejectedRatio):
		return degraded(ReasonRejectedSome)
	case w.Saturated > 0:
		return degraded(ReasonSaturated)
	case atLeast(h.RetryRatio, DegradedRetryRatio):
		return degraded(ReasonRetrying)
	case h.LatencyP95Sec != nil && *h.LatencyP95Sec > SlowDeliverySec:
		return degraded(ReasonSlowDelivery)
	}
	h.State, h.Reason = HealthHealthy, ReasonDelivering
	return h
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Minutes lays an entity's series out for Assess. A nil value is a minute
// nobody observed.
func (e Entity) minutes() []Minute {
	n := len(e.Covered)
	out := make([]Minute, n)
	at := func(name string, i int) float64 {
		if v := e.Values[name]; i < len(v) && v[i] != nil {
			return *v[i]
		}
		return 0
	}
	for i := range out {
		out[i] = Minute{Observed: e.Covered[i] > 0, WindowCounts: WindowCounts{
			// A refused request is at least one event that never got in: counted
			// with the rejected ones, as a floor (one body may have held many).
			Received: at("received", i), Rejected: at("rejected", i) + at("refused", i), Dropped: at("dropped", i),
			DeadLetter: at("deadLetter", i), Delivered: at("delivered", i), Retried: at("retried", i),
			Buried: at("buried", i), Saturated: at("saturated", i)}}
		if i < len(e.lat) {
			out[i].Lat = e.lat[i]
		}
	}
	return out
}

// Health is the health of one stream in a snapshot read over the whole
// retention (Snapshot(k, Retention, ...)): a shorter one cannot say since when,
// nor the last traffic of a quiet stream.
func (s Snapshot) Health(owner, name string) Health {
	in := Input{Now: s.now, Interval: time.Duration(s.IntervalSec * float64(time.Second))}
	for i := range s.Owners {
		if s.Owners[i].Name == owner {
			in.Source = &s.Owners[i]
		}
	}
	for _, e := range s.Entities {
		if e.Owner == owner && e.Name == name {
			in.Present, in.Minutes = e.Present, e.minutes()
			in.Start = time.Unix(s.Window.Start, 0)
		}
	}
	if !s.Configured {
		in.Source = nil
	}
	return Assess(in)
}
