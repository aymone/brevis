package api_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func runNowUI(spy *triggerSpy) *api.UI {
	reads := readsFake{workflows: []postgres.WorkflowSummary{
		{Slug: "daily", Name: "daily", Project: "p", Cron: "0 9 * * *", Timezone: "UTC", HasSchedule: true, Active: true},
		{Slug: "handmade", Name: "handmade", Project: "p"},
		{Slug: "tuned", Name: "tuned", Project: "p", Cron: "0 9 * * *", HasSchedule: true, Active: true},
	}}
	one, every := 0, 0
	defs := mapDefs{one: &one, every: &every, defs: map[string]wf.Workflow{
		"daily":    {Slug: "daily", Schedule: "0 9 * * *", Nodes: []wf.Node{{ID: "a", Run: "echo"}}},
		"handmade": {Slug: "handmade", Nodes: []wf.Node{{ID: "a", Run: "echo"}}},
		"tuned":    {Slug: "tuned", Schedule: "0 9 * * *", Params: []wf.Param{{Name: "full", Type: wf.ParamBool, Default: "false"}}, Nodes: []wf.Node{{ID: "a", Run: "echo"}}},
	}}
	return api.NewUI(reads, defs, nil, spy, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}

func postRun(t *testing.T, ui *api.UI, form url.Values, script bool) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodPost, "/workflows/daily/trigger", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if script {
		r.Header.Set("X-Brevis-Fragment", "trigger")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

// THE DIALOG'S CHOICE REACHES THE SCHEDULER: now is no slot, a listed slot
// is that slot, and "another" is the time typed beside it.
func TestTheRunDialogsChoiceReachesTheScheduler(t *testing.T) {
	for _, c := range []struct {
		name string
		form url.Values
		want *time.Time
	}{
		{"now", url.Values{"when": {"now"}, "slot": {"2026-10-26T09:00:00Z"}}, nil},
		{"a listed slot", url.Values{"when": {"slot"}, "slot": {"2026-10-26T09:00:00Z"}}, ptr(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))},
		{"another", url.Values{"when": {"slot"}, "slot": {"other"}, "slot_at": {"2026-10-20T09:00"}}, ptr(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))},
	} {
		t.Run(c.name, func(t *testing.T) {
			spy := &triggerSpy{}
			if rec := postRun(t, runNowUI(spy), c.form, false); rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			if (spy.slot == nil) != (c.want == nil) || (c.want != nil && !spy.slot.Equal(*c.want)) {
				t.Errorf("slot = %v, want %v", spy.slot, c.want)
			}
		})
	}
}

func ptr(t time.Time) *time.Time { return &t }

// THE DIALOG STAYS OPEN ON A REFUSAL: with the script, a refused slot is the
// scheduler's sentence for the dialog to show; a run made is where to go
// next.
func TestTheRunDialogHearsTheOutcome(t *testing.T) {
	spy := &triggerSpy{err: errors.New(`2026-10-26 10:00 UTC is not a slot of "0 9 * * *"`)}
	rec := postRun(t, runNowUI(spy), url.Values{"when": {"slot"}, "slot": {"2026-10-26T10:00:00Z"}}, true)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "is not a slot") {
		t.Errorf("a refusal answered %d: %s", rec.Code, rec.Body)
	}

	rec = postRun(t, runNowUI(&triggerSpy{}), url.Values{"when": {"now"}}, true)
	var got struct{ Location string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !strings.HasPrefix(got.Location, "/runs/") {
		t.Errorf("a run made answered %q (%v)", rec.Body, err)
	}
}
