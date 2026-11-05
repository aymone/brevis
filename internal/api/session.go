package api

import (
	"net/http"
	"time"

	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/web/i18n"
)

// sessionView is who is looking and in which language: what changes from one
// person to the next, and so is never cached. What belongs to the installation
// is configView.
type sessionView struct {
	// Operator is empty when the installation has no credential: the console
	// is open, and the app shows no sign-out button, as the layout does.
	Operator string `json:"operator"`
	Lang     string `json:"lang"`
}

func (u *UI) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, sessionView{
		Operator: auth.De(r.Context()),
		Lang:     i18n.Negotiate(r),
	})
}

// configView is what the installation is, the same for everyone who opens it:
// its identity, the version it runs and which services it has. The app reads it
// once, before drawing anything.
type configView struct {
	Brand   brandView `json:"brand"`
	Version string    `json:"version"`
	// SQL says whether the SQL tabs exist at all, so the app does not draw a
	// link to a service this installation does not run.
	SQL bool `json:"sql"`
}

// brandView is the installation's identity. CSS is the theme the server
// already writes for the pages (branding.CSS): the app injects it as it is, so
// a client's colours have one source and no second reading of the same file.
type brandView struct {
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Phrase      string `json:"phrase"`
	Logo        string `json:"logo"`
	Attribution string `json:"attribution"`
	CSS         string `json:"css"`
}

func (u *UI) config(w http.ResponseWriter, r *http.Request) {
	logo := u.brand.Logo
	if logo == "" {
		logo = branding.DefaultLogo
	}
	// Fixed for the life of the process (the brand is read at start), so a
	// changed one arrives with the next deploy. no-cache: the app keeps it, but
	// asks again when it starts.
	w.Header().Set("Cache-Control", "no-cache")
	// This endpoint is public (the sign-in screen draws the brand and its words
	// from it), so a stranger gets the brand and nothing about the installation:
	// not the engine's version (it tells an attacker which flaws to try) and not
	// which services exist. A signed-in visitor, or an open console, gets all.
	version, sql := u.version, u.preview.Configured()
	if auth.Anonymous(r.Context()) {
		version, sql = "", false
	}
	writeJSON(w, http.StatusOK, configView{
		Brand: brandView{
			Title:       u.brand.Title,
			Subtitle:    u.brand.Subtitle,
			Phrase:      u.brand.Phrase,
			Logo:        logo,
			Attribution: branding.Attribution,
			CSS:         u.brand.CSS(),
		},
		Version: version,
		SQL:     sql,
	})
}

// projectView is one project as the Projects screen lists it.
type projectView struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Workflows int       `json:"workflows"`
	Runs      int       `json:"runs"`
	CreatedAt time.Time `json:"createdAt"`
}

// projects is the Projects screen's data. The list is always an array, empty
// included, so the app does not have to tell null from none.
func (u *UI) projects(w http.ResponseWriter, r *http.Request) {
	ps, err := u.leitura.Projects(r.Context())
	if err != nil {
		u.log.Error("listing the projects", "error", err)
		fail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	out := make([]projectView, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectView{Slug: p.Slug, Name: p.Name, Workflows: p.Workflows, Runs: p.Runs, CreatedAt: p.CreatedAt})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// messages is the console's catalog in the reader's language, for the React
// app's t(). The same JSON files the server pages translate from: one source
// for the words. no-cache with a validator: a deploy that changes a sentence
// reaches the next load, and an unchanged one is a 304.
func (u *UI) messages(w http.ResponseWriter, r *http.Request) {
	body, etag := i18n.BundleJSON(i18n.Negotiate(r))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	// The language is negotiated from a cookie and Accept-Language.
	w.Header().Add("Vary", "Cookie, Accept-Language")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
