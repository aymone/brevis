package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

// registerRunAPI wires the open run's JSON for the React app. Kept apart from
// Registrar so the screen's routes live beside their handlers.
func (u *UI) registerRunAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/runs/{id}", u.runJSON)
	mux.HandleFunc("POST /api/runs/{id}/cancel", u.cancelJSON)
}

func runFail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// runIDOf reads the {id} of the route; false has already answered.
func (u *UI) runIDOf(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || u.execs == nil {
		runFail(w, http.StatusNotFound, "run not found")
		return id, false
	}
	return id, true
}

// runJSON is the open run, finished by the server: the same RunPage the page
// and its live refresh are drawn from, so the app cannot read a run differently.
func (u *UI) runJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := u.runIDOf(w, r)
	if !ok {
		return
	}
	current, err := u.execs.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		runFail(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		u.log.Error("reading the run", "run", id, "error", err)
		runFail(w, http.StatusInternalServerError, "the run is unavailable")
		return
	}
	// A live run is read again and again: a proxy's copy would be a lie.
	w.Header().Set("Cache-Control", "no-store")
	ctx := i18n.WithLang(r.Context(), i18n.Negotiate(r))
	writeJSON(w, http.StatusOK, pages.NewRunView(ctx, u.runPage(ctx, current), time.Now()))
}

type cancelView struct {
	// Outcome is "canceled" (it was waiting and is stopped) or "requested"
	// (a dispatcher holds it and stops its steps).
	Outcome string `json:"outcome"`
	Status  string `json:"status"`
}

// cancelJSON stops a run, as the page's Cancel does; the answer says what
// became of it and the app reads the run again.
func (u *UI) cancelJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := u.runIDOf(w, r)
	if !ok {
		return
	}
	if u.actions == nil {
		runFail(w, http.StatusNotFound, "run not found")
		return
	}
	got, err := u.actions.Cancel(r.Context(), id, auth.De(r.Context()))
	if errors.Is(err, postgres.ErrRunNotFound) {
		runFail(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		u.log.Error("canceling the run", "run", id, "error", err)
		runFail(w, http.StatusInternalServerError, "the run could not be canceled")
		return
	}
	u.log.Info("run cancel asked", "run", id, "outcome", got.Outcome, "status", got.Status)
	outcome := "requested"
	if got.Outcome == postgres.CancelDone {
		outcome = "canceled"
	}
	writeJSON(w, http.StatusOK, cancelView{Outcome: outcome, Status: string(got.Status)})
}
