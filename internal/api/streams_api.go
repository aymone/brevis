package api

import (
	"net/http"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/metrics"
	"brevis.sh/brevis/web/pages"
)

// streamEntries is the catalog with every stream's configuration attached.
func (u *UI) streamEntries(r *http.Request) ([]postgres.CatalogEntry, error) {
	if u.catalog == nil {
		return nil, nil
	}
	entries, err := u.catalog.Catalog(r.Context())
	if err != nil {
		return nil, err
	}
	configs, err := u.catalog.StreamConfigs(r.Context())
	if err != nil {
		return nil, err
	}
	pages.AttachStreamConfigs(entries, configs)
	return entries, nil
}

// streamsAPI is the Streams list under the page's filter (kind, q).
func (u *UI) streamsAPI(w http.ResponseWriter, r *http.Request) {
	entries, err := u.streamEntries(r)
	if err != nil {
		u.ingestFail(w, "the streams", err)
		return
	}
	v := pages.BuildStreamsWith(entries, pages.ParseStreamFilter(r.URL.Query()), u.streamHealth(u.scrape.Snapshot(metrics.Gateway, metrics.Retention, nil)))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, pages.StreamsAsJSON(v))
}

// streamHealth says a stream's health from a snapshot of its gateway's
// metrics. A stream with no name has nothing to match the gateway's series
// by; one whose gateway no source names, or that no collector serves, keeps
// the static answer unless it can say why it is unknown more precisely.
func (u *UI) streamHealth(snap metrics.Snapshot) func(pages.Stream) pages.StreamHealth {
	return func(s pages.Stream) pages.StreamHealth {
		static := s.Health()
		name := s.Field("name")
		if name == "" {
			return static
		}
		h := snap.Health(s.Gateway, name)
		if h.State == metrics.HealthUnknown && h.Reason == metrics.ReasonSourceNotConfigured && static.Reason == pages.ReasonDestinationUnnamed {
			return static
		}
		return h
	}
}
