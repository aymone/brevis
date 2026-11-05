package api_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	dom "brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/pages"
)

func runAPIUI(e execsFake, a api.Actions) *api.UI {
	return api.NewUI(nil, nil, e, a, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
}

func runJSONOf(t *testing.T, ui *api.UI, path string) (*httptest.ResponseRecorder, pages.RunView) {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var v pages.RunView
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec, v
}

// THE RUN IS ONE JSON, finished: head, phases, why it failed with its streak
// left to the page's reader, the numbered log and the step table.
func TestTheRunComesAsOneFinishedJSON(t *testing.T) {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finished := started.Add(90 * time.Second)
	ui := runAPIUI(execsFake{
		run: dom.Run{ID: logRun, WorkflowSlug: "daily_sales", Status: dom.StatusFailed, TriggerType: "manual",
			CreatedAt: started, StartedAt: &started, FinishedAt: &finished, Err: "boom",
			Params: map[string]string{"b": "2", "a": "1"}},
		logs: []postgres.StepLog{
			{NodeID: "load", MapIndex: dom.Unmapped, Attempt: 0, Status: "failed", ExitCode: exitCode(1), Log: "ok\nError: boom\n"},
			{NodeID: "later", MapIndex: dom.Unmapped, Status: "running"},
		},
	}, nil)
	rec, v := runJSONOf(t, ui, "/api/runs/"+logRun.String())

	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if v.Workflow != "daily_sales" || v.Status != "failed" || v.Trigger == "" {
		t.Errorf("head = %+v", v)
	}
	if v.DurationMs == nil || *v.DurationMs != 90_000 || v.Terminal || v.InFlight {
		t.Errorf("duration %v, terminal %v, in flight %v; a failed run is not terminal (it can retry)", v.DurationMs, v.Terminal, v.InFlight)
	}
	if len(v.Phases) != 4 || v.Phases[3].State != "failed" {
		t.Errorf("phases = %+v", v.Phases)
	}
	if v.Why == nil || v.Why.Title.K != "run.why.step_exit" || v.Why.Title.A["step"] != "load" {
		t.Errorf("why = %+v", v.Why)
	}
	if len(v.Params) != 2 || v.Params[0].Key != "a" {
		t.Errorf("params = %+v; sorted by name", v.Params)
	}
	if len(v.Log.Blocks) != 2 || v.Log.Lines != 2 || !v.Log.Blocks[1].Running || v.Log.Blocks[0].Items[1].Level != "err" {
		t.Errorf("log = %+v", v.Log)
	}
	if len(v.Steps) != 2 || len(v.Alerts) != 0 || v.Wait != nil {
		t.Errorf("steps %d, alerts %d, wait %v", len(v.Steps), len(v.Alerts), v.Wait)
	}
}

// LISTS ARE ARRAYS, never null: a run that has not started has none of them.
func TestARunWithNothingYetHasEmptyListsNotNull(t *testing.T) {
	ui := runAPIUI(execsFake{run: dom.Run{ID: logRun, WorkflowSlug: "w", Status: dom.StatusQueued}}, nil)
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs/"+logRun.String(), nil))
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, k := range []string{"logos", "phases", "auto", "params", "steps", "alerts", "context"} {
		if string(raw[k]) == "null" || raw[k] == nil {
			t.Errorf("%s = %s, want an array", k, raw[k])
		}
	}
	var log struct{ Blocks json.RawMessage }
	_ = json.Unmarshal(raw["log"], &log)
	if string(log.Blocks) != "[]" {
		t.Errorf("log.blocks = %s", log.Blocks)
	}
}

// A WAITING RUN SAYS WHAT HOLDS IT, with the three holders a line has room for.
func TestAQueuedRunSaysWhatHoldsIt(t *testing.T) {
	var holders []postgres.Holder
	for i := 0; i < 5; i++ {
		holders = append(holders, postgres.Holder{RunID: uuid.New(), Workflow: fmt.Sprint("w", i), Since: time.Now()})
	}
	ui := runAPIUI(execsFake{run: dom.Run{ID: logRun, WorkflowSlug: "w", Status: dom.StatusQueued},
		wait: &postgres.Wait{Group: "g", Limit: 2, Holders: holders, Ahead: 1}}, nil)
	_, v := runJSONOf(t, ui, "/api/runs/"+logRun.String())
	if v.Wait == nil || v.Wait.Group != "g" || len(v.Wait.Holders) != 3 || v.Wait.More != 2 || v.Wait.Ahead != 1 {
		t.Errorf("wait = %+v", v.Wait)
	}
}

// FAILURES ARE JSON, never HTML: no such run, a malformed id, a broken read.
func TestTheRunsErrorsAreJSON(t *testing.T) {
	for name, tc := range map[string]struct {
		ui   *api.UI
		path string
		want int
	}{
		"unknown": {runAPIUI(execsFake{err: pgx.ErrNoRows}, nil), "/api/runs/" + logRun.String(), 404},
		"bad id":  {runAPIUI(execsFake{}, nil), "/api/runs/nope", 404},
		"broken":  {runAPIUI(execsFake{err: fmt.Errorf("db down")}, nil), "/api/runs/" + logRun.String(), 500},
	} {
		rec, _ := runJSONOf(t, tc.ui, tc.path)
		var body map[string]string
		if rec.Code != tc.want || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["error"] == "" {
			t.Errorf("%s: %d %s; want %d and {\"error\"}", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func postCancel(ui *api.UI, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

// CANCEL IS JSON: who asked and what became of the run; no redirect.
func TestCancelAnswersWithWhatBecameOfTheRun(t *testing.T) {
	spy := &cancelSpy{}
	rec := postCancel(runAPIUI(execsFake{}, spy), "/api/runs/"+logRun.String()+"/cancel")
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 200 || body["outcome"] != "requested" || body["status"] != "running" || spy.id != logRun {
		t.Errorf("%d %s (canceled %s)", rec.Code, rec.Body.String(), spy.id)
	}
}

func TestCancelFailuresAreJSON(t *testing.T) {
	for path, err := range map[string]error{
		"/api/runs/" + logRun.String() + "/cancel": fmt.Errorf("run: %w", postgres.ErrRunNotFound),
		"/api/runs/nope/cancel":                    nil,
	} {
		rec := postCancel(runAPIUI(execsFake{}, &cancelSpy{err: err}), path)
		var body map[string]string
		if rec.Code != 404 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["error"] == "" {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := postCancel(runAPIUI(execsFake{}, &cancelSpy{err: fmt.Errorf("db down")}), "/api/runs/"+logRun.String()+"/cancel")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("a broken cancel = %d %s", rec.Code, rec.Body.String())
	}
}
