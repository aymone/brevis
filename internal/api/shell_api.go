package api

import (
	"net/http"
	"time"
)

// navBadges are the sidebar's live numbers: the JSON sibling of /nav/runs (an
// HTML fragment) for the React shell. Attention rides along because it is the
// other badge the sidebar draws and is read the same way.
type navBadges struct {
	Failing   int `json:"failing"`
	Running   int `json:"running"`
	Attention int `json:"attention"`
}

func (u *UI) navRunsJSON(w http.ResponseWriter, r *http.Request) {
	var out navBadges
	if u.failing != nil {
		f, err := u.failing.FailingWorkflows(r.Context())
		if err != nil {
			u.log.Error("counting the failing workflows", "path", r.URL.Path, "error", err)
			fail(w, http.StatusInternalServerError, codeUnavailable)
			return
		}
		out.Failing = len(f)
	}
	if u.inFlight != nil {
		n, err := u.inFlight(r.Context())
		if err != nil {
			u.log.Error("counting the runs in flight", "path", r.URL.Path, "error", err)
			fail(w, http.StatusInternalServerError, codeUnavailable)
			return
		}
		out.Running = n
	}
	out.Attention, _ = u.attentionCount(r.Context(), r.URL.Path)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// releaseView is the newest Brevis release, for the version modal and the
// update notice. Notes is HTML the updates package already sanitized.
type releaseView struct {
	Tag       string    `json:"tag"`
	URL       string    `json:"url"`
	Notes     string    `json:"notes"`
	Published time.Time `json:"published"`
	Newer     bool      `json:"newer"`
}

// release answers 204 where no release is known (a development build, or a host
// that cannot reach GitHub): the app then shows the version as plain text. Not a
// 404: that is an error in the browser's console on every page, for something
// that is not one.
func (u *UI) release(w http.ResponseWriter, _ *http.Request) {
	if u.updates != nil && u.version != "" {
		if r, newer, ok := u.updates.Latest(); ok {
			w.Header().Set("Cache-Control", "no-cache")
			writeJSON(w, http.StatusOK, releaseView{Tag: r.Tag, URL: r.URL, Notes: r.Notes, Published: r.Published, Newer: newer})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
