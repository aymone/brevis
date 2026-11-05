package pages

import (
	"encoding/json"
	"testing"
	"time"

	"brevis.sh/brevis/internal/alerts"
	"brevis.sh/brevis/internal/domain/diagnosis"
	"brevis.sh/brevis/internal/domain/run"
)

func wire(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// THE SERVER SAYS WHICH SENTENCE, THE APP SAYS IT: what a run's why, its context note and its alerts send.
func TestTheRunsSentencesAreMessages(t *testing.T) {
	code := 3
	for _, c := range []struct {
		name string
		got  any
		want string
	}{
		{"before any step", whyTitle(&diagnosis.Cause{MapIndex: run.Unmapped}), `{"k":"run.why.run"}`},
		{"a step", whyTitle(&diagnosis.Cause{Step: "load", MapIndex: run.Unmapped}), `{"k":"run.why.step","a":{"step":"load"}}`},
		{"a mapped step that exited", whyTitle(&diagnosis.Cause{Step: "load", MapIndex: 2, ExitCode: &code}), `{"k":"run.why.step_exit","a":{"code":3,"step":"load[2]"}}`},
		{"still running", contextState("running"), `{"k":"run.context.running"}`},
		{"nothing published", contextState("done"), `{"k":"run.context.none"}`},
		{"an alert about a step", alertWording(alerts.Record{Kind: alerts.KindStep, NodeID: "load"}),
			`{"k":"run.alert.waiting","a":{"subject":{"k":"run.alert.step","a":{"step":"load"}}}}`},
	} {
		if got := wire(t, c.got); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// THE AUTO PARAMS ARE SENT AS DATA: an instant, a lateness in seconds, a plain value, and the sentence about each.
func TestTheAutoParamsAreDataAndANote(t *testing.T) {
	started := time.Date(2026, 3, 11, 1, 0, 37, 0, time.UTC)
	r := run.Run{Auto: run.AutoParams{AdjustedAt: started, Date: "2026-03-11", DelaySeconds: 37, StartedAt: &started}}
	items := autoItems(r)
	keys := ""
	for _, it := range items {
		keys += it.Key + " "
	}
	if keys != "adjusted_at date delay started_at previous_error " {
		t.Fatalf("keys = %s", keys)
	}
	if items[0].At == nil || !items[0].At.Equal(started) || items[0].Note.K != "auto.adjusted_at" {
		t.Errorf("adjusted_at = %+v", items[0])
	}
	if items[1].Value != "2026-03-11" || items[2].DelaySeconds == nil || *items[2].DelaySeconds != 37 {
		t.Errorf("date, delay = %+v %+v", items[1], items[2])
	}
	if items[4].Value != "false" {
		t.Errorf("previous_error = %+v", items[4])
	}
}

// A SLOT IS A MESSAGE WITH INSTANTS: the one in the schedule's zone, the window in UTC.
func TestASlotLabelIsAMessageWithInstants(t *testing.T) {
	at := time.Date(2026, 10, 15, 4, 0, 0, 0, time.UTC)
	got := wire(t, slotLabel(Slot{At: at, From: at.Add(-24 * time.Hour)}, "America/Sao_Paulo"))
	want := `{"k":"workflows.slot.option","a":{"from":{"$":"at","layout":"brief","v":"2026-10-14T04:00:00Z","zone":"UTC"},` +
		`"slot":{"$":"at","layout":"brief","v":"2026-10-15T04:00:00Z","zone":"America/Sao_Paulo"},` +
		`"to":{"$":"at","layout":"brief","v":"2026-10-15T04:00:00Z","zone":"UTC"}}}`
	if got != want {
		t.Errorf("%s\nwant %s", got, want)
	}
}

// A CRON THAT HAS NO PLAIN READING IS NOT A SENTENCE: the app shows the expression itself.
func TestCronMsg(t *testing.T) {
	if m := CronMsg("0 4 * * *"); m == nil || wire(t, *m) != `{"k":"cron.daily","a":{"at":"04:00"}}` {
		t.Errorf("daily = %v", m)
	}
	if m := CronMsg("0 4 1 6 *"); m != nil {
		t.Error("a yearly cron has no sentence")
	}
}
