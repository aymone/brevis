package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	dom "brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// queuedPage renders a queued run's page, or its live fragment, with what
// WaitingOn answered.
func queuedPage(t *testing.T, path string, wait *postgres.Wait, waitErr error) (int, string) {
	t.Helper()
	id := uuid.New()
	ui := api.NewUI(nil, nil,
		execsFake{run: dom.Run{ID: id, WorkflowSlug: "daily", Status: dom.StatusQueued},
			wait: wait, waitErr: waitErr},
		nil, nil, nil, branding.Default(), slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	ui.Registrar(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs/"+id.String()+path, nil))
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// Nothing proved the limit holds it, so nothing is said.
func TestAQueuedRunTheLimitDoesNotHoldSaysNothing(t *testing.T) {
	_, body := queuedPage(t, "", nil, nil)
	if strings.Contains(body, "Waiting for") {
		t.Error("the page gives a reason the query did not find")
	}
}

// The question failing is not the page failing: logs and alerts already work
// this way, and a page missing one line beats a blank one.
func TestAFailedWaitQuestionKeepsThePage(t *testing.T) {
	code, body := queuedPage(t, "", nil, io.ErrUnexpectedEOF)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if strings.Contains(body, "Waiting for") {
		t.Error("a reason is shown although the question failed")
	}
}
