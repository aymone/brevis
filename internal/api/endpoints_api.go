package api

import (
	"context"
	"net/http"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

type rebuiltView struct {
	At time.Time `json:"at"`
	// By is the first writer of the table, as the old page said it.
	By string `json:"by"`
}

type endpointRowView struct {
	API   string `json:"api"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Keyed bool   `json:"keyed"`
	About string `json:"about"`
	// Freshness is "stale" (a table it reads is late or stale), "fresh", or ""
	// when the catalog knows none of its tables.
	Freshness string       `json:"freshness"`
	Reads     []string     `json:"reads"`
	Rebuilt   *rebuiltView `json:"rebuilt"`
	// TTL is the cache's time to live in words; "" with no cache on record.
	TTL         string    `json:"ttl"`
	Params      int       `json:"params"`
	Required    int       `json:"required"`
	PublishedAt time.Time `json:"publishedAt"`
}

type endpointsView struct {
	Now         time.Time         `json:"now"`
	Total       int               `json:"total"`
	APIs        int               `json:"apis"`
	Fresh       int               `json:"fresh"`
	Stale       int               `json:"stale"`
	Caches      []string          `json:"caches"`
	Connections []string          `json:"connections"`
	APINames    []string          `json:"apiNames"`
	Rows        []endpointRowView `json:"rows"`
}

func endpointRowOf(ctx context.Context, r pages.EndpointRow) endpointRowView {
	v := endpointRowView{
		API: r.API, Name: r.Name, Path: r.Path, Keyed: r.Keyed, About: r.About, Freshness: r.Freshness,
		Reads: append([]string{}, r.Reads...), Params: len(r.Params), Required: len(r.Required), PublishedAt: r.PublishedAt,
	}
	if r.TTL != "" {
		v.TTL = epDuration(ctx, r.TTL)
	}
	if d := r.Read; d != nil && d.LastWhen != nil {
		v.Rebuilt = &rebuiltView{At: *d.LastWhen, By: pages.WriterLine(*d)}
	}
	return v
}

// apiEndpoints is Data → Endpoints' data: every published endpoint the filter
// keeps (api, state, q), worst first by the data it reads, and the counts of
// every endpoint whatever is shown.
func (u *UI) apiEndpoints(w http.ResponseWriter, r *http.Request) {
	var all []postgres.PublishedEndpoint
	if u.endpoints != nil {
		var err error
		if all, err = u.endpoints.List(r.Context()); err != nil {
			u.log.Error("listing the endpoints", "error", err)
			dataFail(w, http.StatusInternalServerError, "the endpoints are unavailable")
			return
		}
	}
	var entries []postgres.CatalogEntry
	if u.catalog != nil && len(all) > 0 {
		var err error
		if entries, err = u.catalog.Catalog(r.Context()); err != nil {
			u.log.Error("reading the catalog for the endpoints", "error", err)
			dataFail(w, http.StatusInternalServerError, "the endpoints are unavailable")
			return
		}
	}
	now := time.Now()
	s := pages.BuildEndpoints(all, entries, pages.ParseEndpointFilter(r.URL.Query()), now)
	ctx := i18n.WithLang(r.Context(), i18n.Negotiate(r))
	v := endpointsView{
		Now: now, Total: s.Total, APIs: s.APIs, Fresh: s.Fresh, Stale: s.Stale,
		Caches: append([]string{}, s.Caches...), Connections: append([]string{}, s.Connections...),
		APINames: append([]string{}, s.APINames...), Rows: make([]endpointRowView, 0, len(s.Rows)),
	}
	for _, row := range s.Rows {
		v.Rows = append(v.Rows, endpointRowOf(ctx, row))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}
