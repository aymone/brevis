package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// apiGet reads a JSON endpoint into a map and keeps the response.
func apiGet(t *testing.T, ui *api.UI, target string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	return getJSON(t, ui, target, nil)
}

// THE OVERVIEW IS ONE JSON: the window it was asked for (anything unknown is
// 24h), the lists as arrays and the times as RFC3339.
func TestTheOverviewJSONCarriesTheWindowAndArraysNeverNull(t *testing.T) {
	for _, c := range []struct{ query, window, grain string }{
		{"", "24h", "hour"}, {"?window=7d", "7d", "day"}, {"?window=30d", "30d", "day"}, {"?window=1y", "24h", "hour"},
	} {
		rec, body := apiGet(t, overviewUI(overviewReads{}), "/api/overview"+c.query)
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%q: %d, cache %q", c.query, rec.Code, rec.Header().Get("Cache-Control"))
		}
		if body["window"] != c.window || body["grain"] != c.grain {
			t.Errorf("%q: window %v grain %v", c.query, body["window"], body["grain"])
		}
		for _, k := range []string{"buckets", "next", "recent", "events"} {
			if _, ok := body[k].([]any); !ok {
				t.Errorf("%q: %s = %v; want an array", c.query, k, body[k])
			}
		}
		if _, err := time.Parse(time.RFC3339, body["now"].(string)); err != nil {
			t.Errorf("now is not RFC3339: %v", err)
		}
	}
}

// THE NUMBERS AND THE RECENT RUNS come from the same reads as the page; the
// next triggers are computed on the server from the cron.
func TestTheOverviewJSONHasTheIndicatorsTheRunsAndTheNextTriggers(t *testing.T) {
	slot := time.Date(2026, 10, 26, 22, 18, 0, 0, time.UTC)
	rec, body := apiGet(t, overviewUI(overviewReads{
		ind: postgres.Indicators{Succeeded: 8, Failed: 2, Running: 1, Retried: 3, MeanDuration: 1500 * time.Millisecond},
		recent: []postgres.RunSummary{
			{ID: "r1", WorkflowSlug: "daily", Status: "failed", TriggerType: "schedule", Attempt: 1, LogicalDate: &slot,
				CreatedAt: slot, Duration: dur(2 * time.Second), Err: "boom"},
			{ID: "r2", WorkflowSlug: "hourly", Status: "running", CreatedAt: slot},
		},
		schedules: []postgres.ScheduleSummary{
			{WorkflowSlug: "daily", Cron: "0 4 * * *", Timezone: "UTC", Active: true},
			{WorkflowSlug: "off", Cron: "0 4 * * *", Timezone: "UTC", Active: false},
			{WorkflowSlug: "broken", Cron: "not a cron", Timezone: "UTC", Active: true},
		},
	}), "/api/overview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	ind := body["indicators"].(map[string]any)
	if ind["succeeded"] != 8.0 || ind["failed"] != 2.0 || ind["retried"] != 3.0 || ind["meanDurationMs"] != 1500.0 {
		t.Errorf("indicators = %v", ind)
	}
	if body["queued"] != 2.0 {
		t.Errorf("queued = %v", body["queued"])
	}
	recent := body["recent"].([]any)
	first := recent[0].(map[string]any)
	if len(recent) != 2 || first["id"] != "r1" || first["durationMs"] != 2000.0 || first["error"] != "boom" || first["logicalDate"] != "2026-10-26T22:18:00Z" {
		t.Errorf("recent = %v", recent)
	}
	if second := recent[1].(map[string]any); second["durationMs"] != nil || second["logicalDate"] != nil {
		t.Errorf("a run with no duration or slot says null: %v", second)
	}
	next := body["next"].([]any)
	if len(next) != 1 || next[0].(map[string]any)["workflow"] != "daily" || next[0].(map[string]any)["cronText"] == "" {
		t.Errorf("next = %v; want the active, valid schedule only", next)
	}
}

// A FAILURE IS JSON with a status, never the HTML error page.
func TestTheOverviewJSONFailsAsJSON(t *testing.T) {
	ui := api.NewUI(nil, nil, nil, nil, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	rec, body := apiGet(t, ui, "/api/overview")
	if rec.Code != http.StatusServiceUnavailable || body["error"] == nil {
		t.Errorf("%d %v", rec.Code, body)
	}
}

// ATTENTION lists the failing workflows with why, in the order the page uses,
// and the arrays are never null.
func TestTheAttentionJSONListsTheFailingWorkflows(t *testing.T) {
	ui, _ := attentionUI(t,
		failed{slug: "once", ago: time.Hour, streak: 1, log: "fine\nERROR: boom"},
		failed{slug: "again", ago: 2 * time.Hour, streak: 3, log: "ERROR: bad", params: map[string]string{"region": "br", "gone": "x"}},
	)
	rec, body := apiGet(t, ui, "/api/overview/attention")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	items := body["items"].([]any)
	if body["total"] != 2.0 || len(items) != 2 {
		t.Fatalf("total %v, items %v", body["total"], items)
	}
	top := items[0].(map[string]any)
	if top["workflow"] != "again" || top["streak"] != 3.0 || top["rerunnable"] != true {
		t.Errorf("the repeat failure leads: %v", top)
	}
	if p := top["params"].(map[string]any); p["region"] != "br" || len(p) != 1 {
		t.Errorf("a rerun sends the declared params only: %v", p)
	}
	cause := top["cause"].(map[string]any)
	if cause["step"] != "test" || cause["exitCode"] != 1.0 || cause["mapIndex"] != nil {
		t.Errorf("cause = %v", cause)
	}
	if _, ok := body["targets"].([]any); !ok {
		t.Errorf("targets = %v; want an array", body["targets"])
	}
}

func TestTheAttentionJSONIsEmptyWhenAllIsWell(t *testing.T) {
	ui, _ := attentionUI(t)
	_, body := apiGet(t, ui, "/api/overview/attention")
	if body["total"] != 0.0 {
		t.Errorf("total = %v", body["total"])
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("items = %v; want []", body["items"])
	}
	// No metrics source: no stream can be said to be failing, and the array is never null.
	if streams, ok := body["streams"].([]any); !ok || len(streams) != 0 || body["streamsMore"] != 0.0 {
		t.Errorf("streams = %v, more %v; want [] and 0", body["streams"], body["streamsMore"])
	}
}
