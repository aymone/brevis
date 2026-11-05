package metrics

import (
	"math"
	"testing"
	"time"
)

// series builds Input.Minutes: `quiet` observed empty minutes, then the given
// ones, the last being the current minute.
func series(quiet int, ms ...Minute) []Minute {
	out := make([]Minute, 0, quiet+len(ms))
	for i := 0; i < quiet; i++ {
		out = append(out, Minute{Observed: true})
	}
	for _, m := range ms {
		m.Observed = true
		out = append(out, m)
	}
	return out
}

func mn(w WindowCounts) Minute { return Minute{Observed: true, WindowCounts: w} }

// input is a healthy source with the minutes given; the last minute is now.
func input(ms []Minute) Input {
	ok := t0.Add(-2 * time.Second)
	return Input{Now: t0, Interval: 5 * time.Second, Present: true, Start: t0.Add(-time.Duration(len(ms)-1) * time.Minute), Minutes: ms,
		Source: &Owner{Name: "gw", State: "ok", Replicas: 1, Reachable: 1, LastSuccess: &ok}}
}

// flat repeats one minute n times.
func flat(n int, m WindowCounts) []Minute {
	out := make([]Minute, n)
	for i := range out {
		out[i] = mn(m)
	}
	return out
}

func TestAssessUnknownSources(t *testing.T) {
	old := t0.Add(-16 * time.Second) // StaleScrapes x 5s = 15s
	edge := t0.Add(-15 * time.Second)
	cases := []struct {
		name   string
		mutate func(*Input)
		reason string
	}{
		{"no source for the gateway", func(in *Input) { in.Source = nil }, ReasonSourceNotConfigured},
		{"first scrape is a baseline", func(in *Input) { in.Source.State = "waiting" }, ReasonWaiting},
		{"unreachable", func(in *Input) { in.Source.State, in.Source.Reachable = "unreachable", 0 }, ReasonSourceUnreachable},
		{"stale by one second over the bound", func(in *Input) { in.Source.LastSuccess = &old }, ReasonSourceUnreachable},
		{"exactly at the bound is still fresh", func(in *Input) { in.Source.LastSuccess = &edge }, ReasonDelivering},
		{"never answered", func(in *Input) { in.Source.LastSuccess = nil }, ReasonSourceUnreachable},
		{"stream not in the source", func(in *Input) { in.Present = false }, ReasonNotInSource},
		{"nothing observed yet", func(in *Input) { in.Minutes = []Minute{{}, {}} }, ReasonWaiting},
	}
	for _, c := range cases {
		in := input(flat(6, WindowCounts{Received: 100, Delivered: 1}))
		c.mutate(&in)
		h := Assess(in)
		if c.reason == ReasonDelivering {
			if h.State != HealthHealthy {
				t.Errorf("%s: %+v", c.name, h)
			}
			continue
		}
		if h.State != HealthUnknown || h.Reason != c.reason {
			t.Errorf("%s: got %s/%s, want unknown/%s", c.name, h.State, h.Reason, c.reason)
		}
	}
}

// A stale source says unknown even when the stream was failing a moment ago:
// nothing is reported from a source nobody heard from.
func TestAssessStaleSourceNeverKeepsTheLastState(t *testing.T) {
	in := input(flat(10, WindowCounts{Received: 100, Rejected: 100}))
	if h := Assess(in); h.State != HealthFailing {
		t.Fatalf("setup: %+v", h)
	}
	old := t0.Add(time.Hour * -1)
	in.Source.LastSuccess = &old
	if h := Assess(in); h.State != HealthUnknown || h.Reason != ReasonSourceUnreachable {
		t.Errorf("%+v", h)
	}
}

func TestAssessQuiet(t *testing.T) {
	// Traffic 7 minutes ago, silence since.
	ms := series(0, mn(WindowCounts{Received: 50, Delivered: 1}))
	ms = append(ms, flat(7, WindowCounts{})...)
	h := Assess(input(ms))
	if h.State != HealthQuiet || h.Reason != ReasonNoTraffic || h.Since == nil || !h.Since.Equal(t0.Add(-7*time.Minute)) {
		t.Errorf("quiet since the last traffic: %+v since=%v", h, h.Since)
	}
	// Nothing in the whole ring.
	h = Assess(input(flat(30, WindowCounts{})))
	if h.State != HealthQuiet || h.Reason != ReasonNoTrafficSeen || h.Since != nil || h.ObservedMinutes != 30 {
		t.Errorf("quiet with no traffic seen: %+v", h)
	}
	// Traffic inside the window is not quiet; the first silent window is.
	if h := Assess(input(append(flat(1, WindowCounts{Received: 5}), flat(HealthWindow-1, WindowCounts{})...))); h.State != HealthHealthy {
		t.Errorf("traffic at the oldest minute of the window: %+v", h)
	}
	if h := Assess(input(append(flat(1, WindowCounts{Received: 5}), flat(HealthWindow, WindowCounts{})...))); h.State != HealthQuiet {
		t.Errorf("traffic just out of the window: %+v", h)
	}
	// A flush after the traffic stopped is still activity.
	if h := Assess(input(append(flat(10, WindowCounts{}), mn(WindowCounts{Delivered: 1})))); h.State != HealthHealthy {
		t.Errorf("a delivery is activity: %+v", h)
	}
}

func TestAssessHealthyAndDegradedBoundaries(t *testing.T) {
	win := func(w WindowCounts) []Minute { return append(flat(HealthWindow-1, WindowCounts{}), mn(w)) }
	lat := func(n float64, le float64) Minute {
		return Minute{Observed: true, WindowCounts: WindowCounts{Received: 10, Delivered: n},
			Lat: map[float64]float64{le: n, math.Inf(1): n}}
	}
	cases := []struct {
		name  string
		ms    []Minute
		state string
		why   string
	}{
		{"events and deliveries", win(WindowCounts{Received: 1000, Delivered: 5}), HealthHealthy, ReasonDelivering},
		{"dropped on purpose is not a fault", win(WindowCounts{Received: 1000, Dropped: 900, Delivered: 5}), HealthHealthy, ReasonDelivering},
		{"rejected just under 1%", win(WindowCounts{Received: 991, Rejected: 9}), HealthHealthy, ReasonDelivering},
		{"rejected at 1%", win(WindowCounts{Received: 990, Rejected: 10}), HealthDegraded, ReasonRejectedSome},
		{"a couple of rejected on a trickle: counts, no failing claim", win(WindowCounts{Received: 2, Rejected: 3}), HealthDegraded, ReasonRejectedSome},
		{"one dead-lettered record is degraded", win(WindowCounts{Received: 1000, DeadLetter: 1, Delivered: 3}), HealthDegraded, ReasonDeadLettered},
		{"one buried batch is degraded", win(WindowCounts{Received: 1000, Buried: 1, Delivered: 3}), HealthDegraded, ReasonDeadLettered},
		{"a refused request is degraded", win(WindowCounts{Received: 1000, Saturated: 1}), HealthDegraded, ReasonSaturated},
		{"retries under a quarter", win(WindowCounts{Received: 10, Delivered: 4, Retried: 1}), HealthHealthy, ReasonDelivering},
		{"retries at a quarter", win(WindowCounts{Received: 10, Delivered: 3, Retried: 1}), HealthDegraded, ReasonRetrying},
		{"retries under the minimum attempts", win(WindowCounts{Received: 10, Retried: 1}), HealthHealthy, ReasonDelivering},
		{"p95 at the bound", []Minute{lat(10, 30)}, HealthHealthy, ReasonDelivering},
		{"p95 over the bound", []Minute{lat(10, 60)}, HealthDegraded, ReasonSlowDelivery},
		{"slow but under the sample minimum", []Minute{lat(MinLatencySamples-1, 60)}, HealthHealthy, ReasonDelivering},
	}
	for _, c := range cases {
		h := Assess(input(c.ms))
		if h.State != c.state || h.Reason != c.why {
			t.Errorf("%s: got %s/%s (%+v), want %s/%s", c.name, h.State, h.Reason, h.Window, c.state, c.why)
		}
	}
}

// bad is a window of minutes with the same amounts in each of `n` minutes
// followed by clean ones, so the evidence spans minutes.
func spread(bad int, w WindowCounts) []Minute {
	return append(flat(HealthWindow-bad, WindowCounts{Received: 1000, Delivered: 5}), flat(bad, w)...)
}

func TestAssessFailingBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		ms    []Minute
		state string
		why   string
	}{
		// 5 minutes: bad minutes carry rejected r out of 20+r... use 1 bad minute out of 5 -> not failing.
		{"a single bad minute never fails", spread(1, WindowCounts{Received: 0, Rejected: 100000}), HealthDegraded, ReasonRejectedSome},
		{"window majority carried by one minute: the second minute is a minority", []Minute{
			mn(WindowCounts{Rejected: 5000}), mn(WindowCounts{Received: 1000, Rejected: 500}), mn(WindowCounts{Received: 100}), mn(WindowCounts{Received: 100}), mn(WindowCounts{Received: 100})},
			HealthDegraded, ReasonRejectedSome},
		{"rejected majority in two minutes", flat(HealthWindow, WindowCounts{Received: 10, Rejected: 10}), HealthFailing, ReasonRejectedMost},
		{"rejected just under the majority", flat(HealthWindow, WindowCounts{Received: 11, Rejected: 10}), HealthDegraded, ReasonRejectedSome},
		{"majority under the minimum volume", flat(1, WindowCounts{Received: 2, Rejected: 18}), HealthDegraded, ReasonRejectedSome},
		{"majority at the minimum volume, two minutes", append(flat(1, WindowCounts{Rejected: 10}), mn(WindowCounts{Rejected: 10})), HealthFailing, ReasonRejectedMost},
		{"majority one event under the minimum volume", append(flat(1, WindowCounts{Rejected: 10}), mn(WindowCounts{Rejected: 9})), HealthDegraded, ReasonRejectedSome},
		{"dead letter majority", flat(HealthWindow, WindowCounts{Received: 10, DeadLetter: 6}), HealthFailing, ReasonDeadLetterMost},
		{"dead letter just under", flat(HealthWindow, WindowCounts{Received: 10, DeadLetter: 4}), HealthDegraded, ReasonDeadLettered},
		{"dead letter one minute only", spread(1, WindowCounts{Received: 10000, DeadLetter: 10000}), HealthDegraded, ReasonDeadLettered},
		{"all batches buried, two minutes", append(flat(1, WindowCounts{Received: 5, Buried: 1}), mn(WindowCounts{Received: 5, Buried: 1})), HealthFailing, ReasonDeliveryFailing},
		{"one buried batch of one: below the minimum", flat(1, WindowCounts{Received: 5, Buried: 1}), HealthDegraded, ReasonDeadLettered},
		{"half the batches buried", flat(HealthWindow, WindowCounts{Received: 5, Buried: 1, Delivered: 1}), HealthFailing, ReasonDeliveryFailing},
		{"a third buried", flat(HealthWindow, WindowCounts{Received: 5, Buried: 1, Delivered: 2}), HealthDegraded, ReasonDeadLettered},
		{"buffer refusing in 4 of 5 minutes", append(flat(1, WindowCounts{Received: 9}), flat(4, WindowCounts{Received: 9, Saturated: 1})...), HealthFailing, ReasonSaturating},
		{"buffer refusing in 3 of 5 minutes", append(flat(2, WindowCounts{Received: 9}), flat(3, WindowCounts{Received: 9, Saturated: 1})...), HealthDegraded, ReasonSaturated},
	}
	for _, c := range cases {
		h := Assess(input(c.ms))
		if h.State != c.state || h.Reason != c.why {
			t.Errorf("%s: got %s/%s (%+v), want %s/%s", c.name, h.State, h.Reason, h.Window, c.state, c.why)
		}
	}
}

func TestAssessRatiosAreSaidWithTheState(t *testing.T) {
	h := Assess(input(flat(HealthWindow, WindowCounts{Received: 63, Rejected: 37})))
	if h.RejectedRatio == nil || math.Abs(*h.RejectedRatio-0.37) > 1e-9 || h.WindowMinutes != HealthWindow || h.Window.Rejected != 185 {
		t.Errorf("%+v %+v", h, h.Window)
	}
}

// hold is the hysteresis: the state worsens at once and improves only after
// RecoverMinutes minutes whose raw state was better.
func TestAssessHysteresis(t *testing.T) {
	bad := WindowCounts{Received: 1000, Rejected: 1000}
	good := WindowCounts{Received: 1000, Delivered: 5}
	// Enough clean history that the window itself is clean when the bad ends.
	run := func(badMinutes, cleanAfter int) Health {
		ms := flat(20, good)
		ms = append(ms, flat(badMinutes, bad)...)
		ms = append(ms, flat(cleanAfter, good)...)
		return Assess(input(ms))
	}
	// 4 bad minutes then 1 clean: the raw window still holds bad ones.
	if h := run(4, 1); h.State == HealthHealthy {
		t.Errorf("recovered too early: %+v", h)
	}
	// Once the window is clean, the hold still lasts RecoverMinutes-1 minutes.
	// The bad minutes leave the 5-minute window after 5 clean minutes (raw OK),
	// the shown state follows RecoverMinutes-1 minutes later.
	for clean, wantHealthy := range map[int]bool{5: false, 6: false, 7: false, 8: true, 9: true} {
		h := run(2, clean)
		if (h.State == HealthHealthy) != wantHealthy {
			t.Errorf("%d clean minutes after: %+v", clean, h)
		}
	}
	if h := run(2, 6); !h.Recovering || h.State == HealthHealthy {
		t.Errorf("held states say they are recovering: %+v", h)
	}
	// Worsening is immediate: the first bad minute that makes the raw state failing.
	ms := append(flat(20, good), flat(2, bad)...)
	if h := Assess(input(ms)); h.State != HealthDegraded {
		t.Errorf("2 bad minutes in a busy window: %+v", h)
	}
}

// A borderline stream: its window holds one noisy minute (0.7% rejected, healthy)
// or two (1.4%, degraded) depending on the alignment, so the RAW state flips
// every minute. The shown state must not.
func TestAssessDoesNotFlap(t *testing.T) {
	good := WindowCounts{Received: 1000, Delivered: 5}
	var ms []Minute
	changes, prev := 0, ""
	for i := 0; i < 90; i++ {
		if i%4 == 3 {
			ms = append(ms, mn(WindowCounts{Received: 1000, Rejected: 35, Delivered: 5}))
		} else {
			ms = append(ms, mn(good))
		}
		if i < 10 {
			continue
		}
		in := input(ms)
		in.Start = t0.Add(-time.Duration(len(ms)-1) * time.Minute)
		s := Assess(in).State
		if prev != "" && s != prev {
			changes++
		}
		prev = s
	}
	if raw := rawFlips(ms); raw < 10 {
		t.Fatalf("the fixture is not borderline: the raw state flipped %d times", raw)
	}
	if changes > 1 {
		t.Errorf("the state changed %d times", changes)
	}
}

func TestAssessSince(t *testing.T) {
	good := WindowCounts{Received: 1000, Delivered: 5}
	ms := flat(30, good)
	ms = append(ms, flat(3, WindowCounts{Received: 10, Rejected: 10})...)
	h := Assess(input(ms))
	// The shown state began when the raw state turned degraded or failing.
	if h.Since == nil || h.Since.Before(t0.Add(-3*time.Minute)) || h.Since.After(t0) {
		t.Errorf("since %v for %+v", h.Since, h)
	}
	// A state that has held for the whole ring has no beginning to claim.
	if h := Assess(input(flat(30, good))); h.State != HealthHealthy || h.Since != nil {
		t.Errorf("%+v since=%v", h, h.Since)
	}
}

func TestAssessPartialReplicas(t *testing.T) {
	in := input(flat(10, WindowCounts{Received: 1000, Delivered: 5}))
	in.Source.Replicas, in.Source.Reachable, in.Source.State = 3, 2, "partial"
	h := Assess(in)
	if h.State != HealthDegraded || h.Reason != ReasonReplicasPartial || h.Replicas != 3 || h.Reachable != 2 {
		t.Errorf("clean but partial: %+v", h)
	}
	in.Minutes = flat(10, WindowCounts{})
	if h := Assess(in); h.State != HealthDegraded || h.Reason != ReasonReplicasPartial || h.Since != nil {
		t.Errorf("quiet but partial cannot claim quiet: %+v", h)
	}
	// Bad evidence on the replicas heard still wins: it is a floor.
	in.Minutes = flat(10, WindowCounts{Received: 10, Rejected: 10})
	if h := Assess(in); h.State != HealthFailing {
		t.Errorf("failing and partial: %+v", h)
	}
}

// A counter that reset, read through the collector, is a normal increase and
// not a negative one: the health of a stream whose gateway restarted does not
// turn red because its counters went to zero.
func TestHealthAcrossACounterReset(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	del := func(v float64) Sample {
		return Sample{Name: "brevis_gateway_batches_total", Labels: map[string]string{"stream": "s", "sink": "x", "outcome": "delivered"}, Value: v}
	}
	feed(c, ck, tg, []Sample{recv("s", 1000), del(10)})
	for i := 1; i <= 3; i++ {
		ck.t = t0.Add(time.Duration(i*5) * time.Second)
		feed(c, ck, tg, []Sample{recv("s", 1000+float64(i)*100), del(10 + float64(i))})
	}
	// The gateway restarted: every counter is back at a small number.
	ck.t = t0.Add(20 * time.Second)
	feed(c, ck, tg, []Sample{recv("s", 40), del(1)})
	h := c.Snapshot(Gateway, Retention, nil).Health("gw", "s")
	if h.State != HealthHealthy || h.Window.Received != 340 {
		t.Errorf("%+v %+v", h, h.Window)
	}
}

// The collector reads the new series: retried and buried batches and the
// saturated counter reach the health rule.
func TestHealthReadsBatchOutcomesAndSaturation(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	b := func(outcome string, v float64) Sample {
		return Sample{Name: "brevis_gateway_batches_total", Labels: map[string]string{"stream": "s", "sink": "x", "outcome": outcome}, Value: v}
	}
	sat := func(v float64) Sample {
		return Sample{Name: "brevis_gateway_saturated_total", Labels: map[string]string{"stream": "s"}, Value: v}
	}
	feed(c, ck, tg, []Sample{recv("s", 0), b("buried", 0), b("retried", 0), sat(0)})
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, tg, []Sample{recv("s", 10), b("buried", 2), b("retried", 3), sat(4)})
	w := c.Snapshot(Gateway, Retention, nil).Health("gw", "s").Window
	if w.Buried != 2 || w.Retried != 3 || w.Saturated != 4 {
		t.Errorf("%+v", w)
	}
}

func rawFlips(ms []Minute) int {
	flips, prev := 0, ""
	for i := 10; i < len(ms); i++ {
		s := rawHealth(ms[max(0, i-HealthWindow+1) : i+1]).State
		if prev != "" && s != prev {
			flips++
		}
		prev = s
	}
	return flips
}

// A body that never decoded is counted with the rejected events (a floor); a
// wrong key and a saturated request are not, because they have their own say.
func TestRefusedRequestsCountAsRejected(t *testing.T) {
	c, ck := newTest("gw")
	tg := c.targets[0]
	ref := func(reason string, v float64) Sample {
		return Sample{Name: "brevis_gateway_requests_refused_total", Labels: map[string]string{"stream": "s", "reason": reason}, Value: v}
	}
	feed(c, ck, tg, []Sample{recv("s", 0), ref("malformed", 0), ref("unauthorized", 0), ref("saturated", 0)})
	ck.t = t0.Add(5 * time.Second)
	feed(c, ck, tg, []Sample{recv("s", 90), ref("malformed", 10), ref("unauthorized", 50), ref("saturated", 7)})
	h := c.Snapshot(Gateway, Retention, nil).Health("gw", "s")
	if h.Window.Rejected != 10 || h.Window.Saturated != 0 || h.State != HealthDegraded || h.Reason != ReasonRejectedSome {
		t.Errorf("%+v %+v", h, h.Window)
	}
}

// A source that was down for a long stretch leaves minutes nobody observed. They
// are not judged, they are not "quiet", and walking back over them must not fail.
func TestAssessAcrossAGapInTheSeries(t *testing.T) {
	good := WindowCounts{Received: 1000, Delivered: 5}
	ms := flat(10, good)
	ms = append(ms, make([]Minute, 20)...) // 20 unobserved minutes
	ms = append(ms, flat(8, good)...)
	h := Assess(input(ms))
	if h.State != HealthHealthy || h.Since == nil {
		t.Errorf("%+v since=%v", h, h.Since)
	}
	// Only a gap: nothing was judged, so nothing is claimed.
	if h := Assess(input(make([]Minute, 12))); h.State != HealthUnknown || h.Reason != ReasonWaiting {
		t.Errorf("%+v", h)
	}
}
