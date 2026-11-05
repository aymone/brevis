package api

import (
	"net/http"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/metrics"
	"brevis.sh/brevis/web/pages"
)

// streamAPI is one gateway stream: what it declares, its flow, its settings
// and where its refusals go. A stream nobody published is a 404.
func (u *UI) streamAPI(w http.ResponseWriter, r *http.Request) {
	entries, err := u.streamEntries(r)
	if err != nil {
		u.ingestFail(w, "the stream", err)
		return
	}
	s := pages.FindStream(pages.StreamsOf(entries), r.PathValue("gateway"), "/"+r.PathValue("path"))
	if s == nil {
		ingestMissing(w)
		return
	}
	var detail *postgres.TargetDetail
	if s.Sink.URI != "" {
		if detail, err = u.catalog.CatalogTarget(r.Context(), s.Sink.URI); err != nil {
			u.ingestFail(w, "the stream", err)
			return
		}
	}
	v := pages.BuildStream(*s, detail)
	if name := s.Field("name"); name != "" {
		snap, err := u.snapshot(r.Context(), metrics.Gateway, metrics.Retention, &[2]string{s.Gateway, name})
		if err != nil {
			u.log.Warn("the live metrics are unavailable", "error", err)
			snap = (*metrics.Collector)(nil).Snapshot(metrics.Gateway, metrics.Retention, nil)
		}
		h := u.streamHealth(snap)(*s)
		v.Live = &h
	}
	v.SQL = s.Sink.URI != "" && u.dataTools(s.Sink.URI)
	if u.endpoints != nil && s.Sink.URI != "" {
		readers, err := u.endpoints.Reading(r.Context(), s.Sink.URI)
		if err != nil {
			u.ingestFail(w, "the stream", err)
			return
		}
		v.ReadBy = pages.ReadersOf(readers)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, pages.StreamAsJSON(v))
}
