package api_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func jsonRuns(t *testing.T, f runsReads, target string) map[string]any {
	t.Helper()
	rec, body := apiGet(t, runsUI(f), target)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%s: %d, cache %q", target, rec.Code, rec.Header().Get("Cache-Control"))
	}
	return body
}

// THE LIST IS THE PAGE'S: the runs, the total, the paging and the counts per
// state the three choices show -- running is everything unfinished.
func TestTheRunsJSONHasThePageTheTotalAndTheCounts(t *testing.T) {
	slot := time.Date(2026, 10, 26, 22, 18, 0, 0, time.UTC)
	f := runsReads{
		runs:   []postgres.RunSummary{{ID: "a", WorkflowSlug: "daily", Status: "failed", TriggerType: "schedule", LogicalDate: &slot, CreatedAt: slot, Duration: dur(time.Second), Err: "boom"}},
		counts: map[string]int{"failed": 3, "running": 1, "queued": 2, "success": 10, "canceled": 1},
		total:  60,
	}
	body := jsonRuns(t, f, "/api/runs?state=failed&page=2")
	if body["total"] != 60.0 || body["page"] != 2.0 || body["perPage"] != 25.0 || body["pages"] != 3.0 {
		t.Errorf("paging = %v", body)
	}
	c := body["counts"].(map[string]any)
	if c["all"] != 17.0 || c["going"] != 3.0 || c["failed"] != 3.0 {
		t.Errorf("counts = %v", c)
	}
	if bs := body["byState"].(map[string]any); bs["canceled"] != 1.0 {
		t.Errorf("byState = %v", bs)
	}
	run := body["runs"].([]any)[0].(map[string]any)
	if run["id"] != "a" || run["durationMs"] != 1000.0 || run["error"] != "boom" || run["logos"] == nil {
		t.Errorf("run = %v", run)
	}
	if body["filter"].(map[string]any)["state"] != "failed" {
		t.Errorf("filter = %v", body["filter"])
	}
}

// NO STATE IN THE ADDRESS is running while anything is in flight, else all; an
// explicit "all" is all even then. The filter says which one it applied.
func TestTheRunsJSONDefaultsToRunningWhileSomethingIsInFlight(t *testing.T) {
	busy := runsReads{counts: map[string]int{"running": 1, "success": 4}}
	if st := jsonRuns(t, busy, "/api/runs")["filter"].(map[string]any)["state"]; st != "running" {
		t.Errorf("default with a run in flight = %v", st)
	}
	if st := jsonRuns(t, busy, "/api/runs?state=all")["filter"].(map[string]any)["state"]; st != "" {
		t.Errorf("state=all = %v", st)
	}
	idle := runsReads{counts: map[string]int{"success": 4}}
	if st := jsonRuns(t, idle, "/api/runs")["filter"].(map[string]any)["state"]; st != "" {
		t.Errorf("default with nothing in flight = %v", st)
	}
}

// A STATE OUTSIDE THE MACHINE, a bad page and a half-pasted period are no
// filter, not an error; the lists are arrays when empty.
func TestTheRunsJSONForgivesABadFilterAndNeverSendsNull(t *testing.T) {
	var asked postgres.RunFilter
	body := jsonRuns(t, runsReads{asked: &asked, listed: &asked}, "/api/runs?state=xpto&page=-4&from=yesterday&workflow=daily")
	f := body["filter"].(map[string]any)
	if f["state"] != "" || f["from"] != "" || f["to"] != "" || f["workflow"] != "daily" || body["page"] != 1.0 {
		t.Errorf("filter = %v, page %v", f, body["page"])
	}
	if runs, ok := body["runs"].([]any); !ok || len(runs) != 0 {
		t.Errorf("runs = %v; want []", body["runs"])
	}
	if asked.Workflow != "daily" || asked.From != nil {
		t.Errorf("the read was filtered by %+v", asked)
	}
}

func TestTheRunsJSONReadsThePeriod(t *testing.T) {
	var asked postgres.RunFilter
	body := jsonRuns(t, runsReads{listed: &asked}, "/api/runs?state=all&from=2026-10-26T10:00:00Z&to=2026-10-26T11:00:00Z")
	if asked.From == nil || asked.To == nil || body["filter"].(map[string]any)["period"] == "" {
		t.Errorf("period not read: %+v %v", asked, body["filter"])
	}
}

type brokenRuns struct{ runsReads }

func (brokenRuns) CountRuns(context.Context, postgres.RunFilter) (int, error) {
	return 0, errors.New("db down")
}

// A READ THAT FAILS is a JSON error with a status, not the HTML error page.
func TestTheRunsJSONFailsAsJSON(t *testing.T) {
	ui := api.NewUI(brokenRuns{}, nil, nil, nil, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	rec, body := apiGet(t, ui, "/api/runs")
	if rec.Code != http.StatusInternalServerError || body["error"] == nil {
		t.Errorf("%d %v", rec.Code, body)
	}
}
