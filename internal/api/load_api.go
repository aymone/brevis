package api

import (
	"net/http"
	"slices"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/pages"
)

// loadAPI is one step landing on one destination. A step that writes several
// asks which (?target=): with none chosen the answer lists the choices and has
// no detail. A step that writes nothing, or not that destination, is a 404.
func (u *UI) loadAPI(w http.ResponseWriter, r *http.Request) {
	workflow, step := r.PathValue("workflow"), r.PathValue("step")
	entries, configs, err := u.stepEntries(r.Context())
	if err != nil {
		u.ingestFail(w, "the load", err)
		return
	}
	var uris []string
	for _, e := range entries {
		for _, wr := range e.Writers {
			if wr.Gateway == nil && wr.Workflow == workflow && wr.Node == step && !slices.Contains(uris, e.Target) {
				uris = append(uris, e.Target)
			}
		}
	}
	target := r.URL.Query().Get("target")
	if target == "" && len(uris) == 1 {
		target = uris[0]
	}
	if len(uris) == 0 || target != "" && !slices.Contains(uris, target) {
		ingestMissing(w, "that load")
		return
	}

	load := &pages.LoadView{Workflow: workflow, Step: step}
	if target != "" {
		d, err := u.catalog.CatalogTarget(r.Context(), target)
		if err != nil {
			u.ingestFail(w, "the load", err)
			return
		}
		if d == nil {
			ingestMissing(w, "that load")
			return
		}
		*load = pages.BuildLoad(*d, workflow, step, time.Now())
		load.Read = u.rowsRead(r.Context(), *load, step)
		if err := u.targetTools(r, &load.TargetView, target); err != nil {
			u.ingestFail(w, "the load", err)
			return
		}
		load.Source = pages.SourceOf(configs[postgres.StepKey{Workflow: workflow, Node: step, Target: target}])
		// Run now, as the workflows list decides it.
		if u.actions != nil && u.defs != nil {
			if def, err := u.defs.Definition(r.Context(), workflow); err == nil {
				load.Run = "now"
				if len(def.Params) > 0 {
					load.Run = "params"
				}
			}
		}
	}
	if len(uris) > 1 {
		for _, uri := range uris {
			load.Choices = append(load.Choices, pages.LoadChoice{URI: uri, Current: uri == target})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, pages.LoadAsJSON(ingestCtx(r), *load))
}
