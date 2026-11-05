package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	sch "brevis.sh/brevis/internal/domain/schedule"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// knownDefs is a catalog that, unlike mapDefs, refuses a workflow it lacks.
type knownDefs map[string]wf.Workflow

func (d knownDefs) Definition(_ context.Context, slug string) (wf.Workflow, error) {
	if w, ok := d[slug]; ok {
		return w, nil
	}
	return wf.Workflow{}, errors.New("no such workflow")
}
func (d knownDefs) Definitions(context.Context) (map[string]wf.Workflow, error) { return d, nil }

func triggerDefs() knownDefs {
	return knownDefs{
		"plain": {Slug: "plain", Nodes: []wf.Node{{ID: "a", Run: "echo"}}},
		"tuned": {Slug: "tuned", Schedule: "0 9 * * *", Nodes: []wf.Node{{ID: "a", Run: "echo"}}, Params: []wf.Param{
			{Name: "region", Type: wf.ParamString, Enum: []string{"br", "us"}, Default: "br"},
			{Name: "limit", Type: wf.ParamInteger, Default: "10"},
			{Name: "full", Type: wf.ParamBool, Default: "false"},
			{Name: "layers", Type: "list|string", Enum: []string{"raw", "mart"}, Default: "raw"},
			{Name: "tables", Type: "list|string"},
			{Name: "token", Type: wf.ParamString},
		}},
	}
}

func triggerAPIUI(spy api.Actions) *api.UI {
	return api.NewUI(readsFake{}, triggerDefs(), nil, spy, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
}

// foldedSpy is a scheduler whose idempotency key already holds a run.
type foldedSpy struct{ triggerSpy }

func (foldedSpy) Trigger(context.Context, string, time.Time, map[string]string, *time.Time) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func triggerPost(t *testing.T, ui *api.UI, slug, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodPost, "/api/workflows/"+slug+"/trigger", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

type refusal struct {
	Error  string
	Fields map[string]map[string]any
}

func refusalOf(t *testing.T, rec *httptest.ResponseRecorder) refusal {
	t.Helper()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	var got refusal
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

// A RUN IS 201 WITH WHERE TO FOLLOW IT, and the values reach the scheduler as
// the strings every param travels as: a list joined by commas, blanks dropped.
func TestTheTriggerAPICreatesARun(t *testing.T) {
	spy := &triggerSpy{}
	rec := triggerPost(t, triggerAPIUI(spy), "tuned",
		`{"params":{"region":"us","limit":25,"full":true,"layers":["raw","mart"],"tables":["a"," b",""],"token":"x"},"from_run":"6f1c0f6e-3d0a-4c35-9a3b-1c2d3e4f5a6b"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct{ RunID, Href string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.RunID == "" || got.Href != "/runs/"+got.RunID {
		t.Errorf("body = %s (%v)", rec.Body, err)
	}
	want := map[string]string{"region": "us", "limit": "25", "full": "true", "layers": "raw,mart", "tables": "a,b", "token": "x"}
	for k, v := range want {
		if spy.got[k] != v {
			t.Errorf("param %s = %q, want %q", k, spy.got[k], v)
		}
	}
	if spy.slot != nil {
		t.Errorf("slot = %v, want none", spy.slot)
	}
}

// NO PARAMS, NO BODY: the defaults run, as an empty form does.
func TestTheTriggerAPIRunsWithDefaults(t *testing.T) {
	spy := &triggerSpy{}
	if rec := triggerPost(t, triggerAPIUI(spy), "plain", ""); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if !spy.called {
		t.Error("nothing was triggered")
	}
}

// EVERY REFUSED VALUE IS A CODE BESIDE ITS FIELD, with the data that makes
// the sentence, and nothing is triggered. The server says no sentence at all.
func TestTheTriggerAPISaysWhichFieldsAreWrong(t *testing.T) {
	spy := &triggerSpy{}
	rec := triggerPost(t, triggerAPIUI(spy), "tuned",
		`{"params":{"region":"mars","limit":"ten","full":"maybe","layers":["raw","raw"],"tables":["a b;"],"ghost":1}}`)
	got := refusalOf(t, rec)
	for field, want := range map[string]map[string]any{
		"region": {"code": "not_one_of", "value": "mars", "allowed": []any{"br", "us"}},
		"limit":  {"code": "not_integer", "value": "ten"},
		"full":   {"code": "not_boolean", "value": "maybe"},
		"layers": {"code": "item_duplicate", "value": "raw"},
		"tables": {"code": "unsafe_chars", "value": "a b;", "item": float64(1)},
		"ghost":  {"code": "undeclared", "value": "ghost"},
		"token":  {"code": "required"},
	} {
		if !reflect.DeepEqual(got.Fields[field], want) {
			t.Errorf("fields[%s] = %v, want %v", field, got.Fields[field], want)
		}
	}
	if got.Error != "invalid_params" || spy.called {
		t.Errorf("error = %q, called = %v", got.Error, spy.called)
	}
	for _, old := range []string{"is not an integer", "accepted values", "shell interprets", "does not declare", "no default", "appears more than once", "invalid value"} {
		if strings.Contains(rec.Body.String(), old) {
			t.Errorf("the response speaks English (%q): %s", old, rec.Body)
		}
	}
}

// A PATTERN MISMATCH AND AN ITEM'S POSITION TRAVEL AS DATA.
func TestTheTriggerAPISaysThePatternAndTheItem(t *testing.T) {
	defs := knownDefs{"p": {Slug: "p", Nodes: []wf.Node{{ID: "a", Run: "echo"}}, Params: []wf.Param{
		{Name: "code", Type: wf.ParamString, Pattern: "^[a-z]+$", Default: "ok"},
		{Name: "days", Type: "list|integer"},
	}}}
	ui := api.NewUI(readsFake{}, defs, nil, &triggerSpy{}, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
	got := refusalOf(t, triggerPost(t, ui, "p", `{"params":{"code":"A1","days":["1","x"]}}`))
	if want := (map[string]any{"code": "no_match", "value": "A1", "pattern": "^[a-z]+$"}); !reflect.DeepEqual(got.Fields["code"], want) {
		t.Errorf("code = %v", got.Fields["code"])
	}
	if want := (map[string]any{"code": "not_integer", "value": "x", "item": float64(2)}); !reflect.DeepEqual(got.Fields["days"], want) {
		t.Errorf("days = %v", got.Fields["days"])
	}
	got = refusalOf(t, triggerPost(t, ui, "p", `{"params":{"days":"1,,2"}}`))
	if want := (map[string]any{"code": "item_empty", "item": float64(2)}); !reflect.DeepEqual(got.Fields["days"], want) {
		t.Errorf("days = %v", got.Fields["days"])
	}
}

// A VALUE THAT IS NO VALUE (an object, a list of lists) is that field's mistake.
func TestTheTriggerAPIRefusesAShapeNoParamHas(t *testing.T) {
	got := refusalOf(t, triggerPost(t, triggerAPIUI(&triggerSpy{}), "tuned", `{"params":{"region":{"a":1},"tables":[["x"]],"token":"t"}}`))
	if got.Fields["region"]["code"] != "bad_value_type" || got.Fields["tables"]["code"] != "nested_list" {
		t.Errorf("fields = %v", got.Fields)
	}
}

// A LIST MAY BE EMPTY (none of them is a choice) but a scalar with no default
// may not.
func TestTheTriggerAPIAllowsAnEmptyList(t *testing.T) {
	spy := &triggerSpy{}
	rec := triggerPost(t, triggerAPIUI(spy), "tuned", `{"params":{"layers":[],"tables":"","token":"t"}}`)
	if rec.Code != http.StatusCreated || spy.got["layers"] != "" {
		t.Fatalf("status = %d, layers = %q: %s", rec.Code, spy.got["layers"], rec.Body)
	}
}

// THE SLOT IS READ AS THE OLD FORM READ IT: an RFC 3339 instant, or a
// datetime-local in the schedule's zone; an unreadable one is the slot field's.
func TestTheTriggerAPIReadsTheSlot(t *testing.T) {
	spy := &triggerSpy{}
	rec := triggerPost(t, triggerAPIUI(spy), "tuned", `{"params":{"token":"t"},"slot":"2026-10-26T09:00:00Z"}`)
	if rec.Code != http.StatusCreated || spy.slot == nil || !spy.slot.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("status = %d, slot = %v: %s", rec.Code, spy.slot, rec.Body)
	}
	spy = &triggerSpy{}
	got := refusalOf(t, triggerPost(t, triggerAPIUI(spy), "tuned", `{"params":{"token":"t"},"slot":"yesterday"}`))
	if want := (map[string]any{"code": "slot_not_a_time", "value": "yesterday"}); !reflect.DeepEqual(got.Fields["slot"], want) || spy.called {
		t.Errorf("fields = %v, called = %v", got.Fields, spy.called)
	}
}

// THE SCHEDULER'S REFUSAL OF A SLOT IS THE SLOT FIELD'S, as codes with the data.
func TestTheTriggerAPIRelaysTheSchedulersRefusal(t *testing.T) {
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	near := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		err  error
		want map[string]any
	}{
		{&sch.SlotError{Code: sch.SlotNotOnCron, Slot: at, Cron: "0 9 * * *", Nearest: near, Zone: "UTC"},
			map[string]any{"code": "slot_not_on_schedule", "slot": "2026-10-26T10:00:00Z", "cron": "0 9 * * *", "zone": "UTC", "nearest": "2026-10-26T09:00:00Z"}},
		{&sch.SlotError{Code: sch.SlotInFuture, Slot: at}, map[string]any{"code": "slot_in_future", "slot": "2026-10-26T10:00:00Z"}},
		{&sch.SlotError{Code: sch.SlotNoSchedule, Workflow: "tuned"}, map[string]any{"code": "slot_no_schedule"}},
	} {
		rec := triggerPost(t, triggerAPIUI(&triggerSpy{err: c.err}), "tuned", `{"params":{"token":"t"},"slot":"2026-10-26T10:00:00Z"}`)
		got := refusalOf(t, rec)
		if !reflect.DeepEqual(got.Fields["slot"], c.want) || got.Error != "invalid_params" {
			t.Errorf("%v: fields = %v", c.err, got.Fields)
		}
		if strings.Contains(rec.Body.String(), "nearest before") || strings.Contains(rec.Body.String(), "in the future") {
			t.Errorf("the response speaks English: %s", rec.Body)
		}
	}
}

// ANY OTHER FAILURE OF THE SCHEDULER IS THE SERVER'S: a code, and its text stays in the log.
func TestTheTriggerAPIAnswers500ToAnUnexpectedSchedulerFailure(t *testing.T) {
	rec := triggerPost(t, triggerAPIUI(&triggerSpy{err: errors.New("nope")}), "plain", `{}`)
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusInternalServerError || got["error"] != "unavailable" || strings.Contains(rec.Body.String(), "nope") {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

// TWO CLICKS IN A SECOND ARE ONE RUN: the second has no run to go to, so it
// is told so (409) instead of being sent nowhere.
func TestTheTriggerAPISaysWhenTheClickFoldedIntoAnother(t *testing.T) {
	rec := triggerPost(t, triggerAPIUI(&foldedSpy{}), "plain", `{}`)
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusConflict || got["error"] != "run_just_started" {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

// AN UNKNOWN WORKFLOW IS 404 AND A BROKEN BODY 400, both as JSON, never HTML.
func TestTheTriggerAPIAnswersJSONToWhatItCannotRun(t *testing.T) {
	for _, c := range []struct {
		slug, body, code string
		status           int
	}{
		{"ghost", `{}`, "not_found", http.StatusNotFound},
		{"plain", `{"params":`, "invalid_request", http.StatusBadRequest},
		{"plain", `{"from_run":"nope"}`, "invalid_request", http.StatusBadRequest},
	} {
		spy := &triggerSpy{}
		rec := triggerPost(t, triggerAPIUI(spy), c.slug, c.body)
		var got map[string]string
		if rec.Code != c.status || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["error"] != c.code || spy.called {
			t.Errorf("%s %s: status = %d, body = %s, called = %v", c.slug, c.body, rec.Code, rec.Body, spy.called)
		}
	}
}

// THE FORM THE DRAWER DRAWS IS THE WORKFLOW'S OWN JSON: enum and pattern per
// param, and the schedule's slots, worded.
func TestTheWorkflowJSONCarriesWhatTheRunDrawerNeeds(t *testing.T) {
	var asked *time.Time
	reads := workflowReads{
		readsFake: readsFake{workflows: []postgres.WorkflowSummary{{Slug: "tuned", Project: "p", Cron: "0 9 * * *", Timezone: "UTC", HasSchedule: true, Active: true}}},
		day:       &asked,
	}
	ui := api.NewUI(reads, defsFake{w: triggerDefs()["tuned"]}, nil, &triggerSpy{}, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler))
	_, body := get(t, ui, "/api/workflows/tuned")
	var got struct {
		Params []struct {
			Name    string
			Enum    []string
			Pattern string
		}
		Slots []struct {
			At    string
			Label struct{ K string }
		}
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Slots) != 14 || got.Slots[0].Label.K != "workflows.slot.option" {
		t.Errorf("slots = %v", got.Slots)
	}
	if len(got.Params) != 6 || len(got.Params[0].Enum) != 2 || got.Params[1].Enum == nil {
		t.Errorf("params = %+v", got.Params)
	}
}
