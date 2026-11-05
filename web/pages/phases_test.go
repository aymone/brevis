package pages

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/domain/run"
)

var created = time.Date(2026, 10, 15, 4, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time { t := created.Add(d); return &t }

// states is the strip as "key:state" pairs, the shape every case asserts.
func states(ps []Phase) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Key+":"+p.State)
	}
	return strings.Join(out, " ")
}

// detail is a phase's detail as "kind:ms" (a duration) or "kind" -- the shape every case asserts.
func detail(p Phase) string {
	d := p.Detail
	if d == nil {
		return ""
	}
	if d.Ms != nil {
		return fmt.Sprintf("%s:%d", d.Kind, *d.Ms)
	}
	if d.State != "" {
		return d.Kind + ":" + d.State
	}
	return d.Kind
}

// A RUN'S PHASES COME FROM ITS OWN TIMES, AND ITS STATUS SAYS WHICH IS NOW.
// One case per state the strip draws differently.
func TestPhasesFollowTheRunsTimes(t *testing.T) {
	now := created.Add(10 * time.Minute)
	for _, c := range []struct {
		name    string
		r       run.Run
		want    string
		details map[string]string
	}{
		{"queued", run.Run{Status: run.StatusQueued, CreatedAt: created},
			"scheduled:done started:current running:pending finished:pending",
			map[string]string{"started": "waiting:600000"}},
		{"running", run.Run{Status: run.StatusRunning, CreatedAt: created, StartedAt: at(2 * time.Second)},
			"scheduled:done started:done running:current finished:pending",
			map[string]string{"started": "late:2000", "running": "ran:598000"}},
		{"succeeded", run.Run{Status: run.StatusSuccess, CreatedAt: created, StartedAt: at(2 * time.Second), FinishedAt: at(92 * time.Second)},
			"scheduled:done started:done running:done finished:done",
			map[string]string{"running": "ran:90000", "finished": "ended:success"}},
		{"failed", run.Run{Status: run.StatusFailed, CreatedAt: created, StartedAt: at(time.Second), FinishedAt: at(31 * time.Second)},
			"scheduled:done started:done running:done finished:failed",
			map[string]string{"running": "ran:30000", "finished": "ended:failed"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ps := PhasesOf(c.r, now)
			if got := states(ps); got != c.want {
				t.Fatalf("phases = %s\nwant     %s", got, c.want)
			}
			for _, p := range ps {
				if want, ok := c.details[p.Key]; ok && detail(p) != want {
					t.Errorf("%s says %q, want %q", p.Key, detail(p), want)
				}
			}
		})
	}
}

// A SCHEDULED RUN NAMES ITS SLOT; one triggered by hand says so instead.
func TestTheFirstPhaseSaysHowTheRunWasMade(t *testing.T) {
	slot := created
	ps := PhasesOf(run.Run{Status: run.StatusQueued, CreatedAt: created, LogicalDate: &slot, TriggerType: "schedule"}, created)
	if ps[0].Label.K != "run.phase.scheduled" || detail(ps[0]) != "slot" || !ps[0].Detail.At.Equal(slot) {
		t.Errorf("a scheduled run's first phase is %+v", ps[0])
	}
	// How late it started is counted from the slot, as its `delay` auto param
	// is: created 1s after the slot and started 3s after it is 3s late.
	ps = PhasesOf(run.Run{Status: run.StatusSuccess, CreatedAt: created.Add(time.Second), LogicalDate: &slot,
		StartedAt: at(3 * time.Second), FinishedAt: at(4 * time.Second)}, created)
	if detail(ps[1]) != "late:3000" {
		t.Errorf("a scheduled run started %q after its slot, want late:3000", detail(ps[1]))
	}
	ps = PhasesOf(run.Run{Status: run.StatusQueued, CreatedAt: created, TriggerType: "manual"}, created)
	if ps[0].Label.K != "run.phase.triggered" || detail(ps[0]) != "at" {
		t.Errorf("a manual run's first phase is %+v", ps[0])
	}
}
