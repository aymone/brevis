package api

import (
	"context"
	"net/http"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/pages"
)

// The Ingest screens' JSON (loads.go and streams.go in the React app). Each
// handler reuses the read its page handler makes; the words are written here in
// the reader's language, and a failure is {"error": "<code>"} with a status, never HTML.

func (u *UI) ingestFail(w http.ResponseWriter, what string, err error) {
	u.log.Error("reading "+what, "error", err)
	fail(w, http.StatusInternalServerError, codeUnavailable)
}

func ingestMissing(w http.ResponseWriter) {
	fail(w, http.StatusNotFound, codeNotFound)
}

// stepEntries is the catalog with every step's configuration attached, the
// read the loads list and a load share. Without a catalog there is nothing.
func (u *UI) stepEntries(ctx context.Context) ([]postgres.CatalogEntry, map[postgres.StepKey]*postgres.WriterConfig, error) {
	if u.catalog == nil {
		return nil, nil, nil
	}
	entries, err := u.catalog.Catalog(ctx)
	if err != nil {
		return nil, nil, err
	}
	configs, err := u.catalog.StepConfigs(ctx)
	if err != nil {
		return nil, nil, err
	}
	pages.AttachStepConfigs(entries, configs)
	return entries, configs, nil
}

// loadsAPI is the Batch loads list: the rows the filter keeps and the counts
// beside them. The filter is the page's own (status, source, q, legacy), so the
// app refetches with it where the page asked for a fragment.
func (u *UI) loadsAPI(w http.ResponseWriter, r *http.Request) {
	entries, _, err := u.stepEntries(r.Context())
	if err != nil {
		u.ingestFail(w, "the loads", err)
		return
	}
	f := pages.ParseDataFilter(r.URL.Query())
	f.Path = "/ingest/loads"
	v := pages.BuildLoads(entries, f, time.Now())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, pages.LoadsAsJSON(v))
}

// registerIngestAPI wires the Ingest screens' JSON.
func (u *UI) registerIngestAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ingest/loads", u.loadsAPI)
	mux.HandleFunc("GET /api/ingest/loads/{workflow}/{step}", u.loadAPI)
	mux.HandleFunc("GET /api/ingest/streams", u.streamsAPI)
	mux.HandleFunc("GET /api/ingest/streams/{gateway}/{path...}", u.streamAPI)
}
