package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	dom "brevis.sh/brevis/internal/domain/run"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// failingList answers the failing workflows, as RunRepo would.
type failingList struct{ list []postgres.FailingWorkflow }

func (f failingList) FailingWorkflows(context.Context) ([]postgres.FailingWorkflow, error) {
	return f.list, nil
}
func (f failingList) FailureStreak(context.Context, string) (int, error) { return 0, nil }

// attentionExecs answers each failed run and its logs, and counts the logs
// read.
type attentionExecs struct {
	execsFake
	runs  map[uuid.UUID]dom.Run
	logs  map[uuid.UUID][]postgres.StepLog
	reads *int
}

func (e attentionExecs) Get(_ context.Context, id uuid.UUID) (dom.Run, error) {
	if r, ok := e.runs[id]; ok {
		return r, nil
	}
	return dom.Run{}, pgx.ErrNoRows
}
func (e attentionExecs) LogsDaRun(_ context.Context, id uuid.UUID) ([]postgres.StepLog, error) {
	*e.reads++
	return e.logs[id], nil
}

type failed struct {
	slug   string
	ago    time.Duration
	streak int
	log    string
	params map[string]string
}

func attentionUI(t *testing.T, fs ...failed) (*api.UI, *int) {
	t.Helper()
	return attentionUIWith(t, nil, fs...)
}

// attentionUIWith also has runs in flight; the workflow every failure belongs
// to declares one param, `region`.
func attentionUIWith(t *testing.T, inFlight []postgres.RunSummary, fs ...failed) (*api.UI, *int) {
	t.Helper()
	reads := 0
	e := attentionExecs{runs: map[uuid.UUID]dom.Run{}, logs: map[uuid.UUID][]postgres.StepLog{}, reads: &reads}
	var list []postgres.FailingWorkflow
	for _, f := range fs {
		id := uuid.New()
		list = append(list, postgres.FailingWorkflow{Slug: f.slug, RunID: id, FailedAt: time.Now().Add(-f.ago), Streak: f.streak})
		e.runs[id] = dom.Run{ID: id, WorkflowSlug: f.slug, Status: dom.StatusFailed, Params: f.params}
		code := 1
		e.logs[id] = []postgres.StepLog{{NodeID: "test", MapIndex: dom.Unmapped, Status: "failed", ExitCode: &code, Log: f.log}}
	}
	def := defsFake{w: wf.Workflow{Params: []wf.Param{{Name: "region", Type: wf.ParamString}}}}
	ui := api.NewUI(overviewReads{inFlight: inFlight, asked: &struct {
		span  time.Duration
		grain postgres.Grain
		n     int
	}{}}, def, e, &triggerSpy{}, nil, nil, branding.Default(), slog.New(slog.DiscardHandler)).WithFailing(failingList{list})
	return ui, &reads
}

func postTrigger(t *testing.T, ui *api.UI, slug string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ui.Registrar(mux)
	r := httptest.NewRequest(http.MethodPost, "/workflows/"+slug+"/trigger", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}
