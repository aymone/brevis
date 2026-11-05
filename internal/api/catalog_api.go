package api

import (
	"net/http"
	"strings"
	"time"

	"brevis.sh/brevis/web/pages"
)

// registerCatalogAPI wires the JSON the React app's Data screens read: the
// catalog, one destination, the endpoints and one endpoint with its Try it.
// Every one answers {"error": "<code>"} with a status on failure, never a page.
func (u *UI) registerCatalogAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog", u.apiCatalog)
	mux.HandleFunc("GET /api/catalog/target", u.apiCatalogTarget)
	mux.HandleFunc("GET /api/endpoints", u.apiEndpoints)
	mux.HandleFunc("GET /api/endpoints/{api}/{name}", u.apiEndpoint)
	mux.HandleFunc("POST /api/endpoints/{api}/{name}/call", u.apiEndpointCall)
}

// dataFail answers a failure of the Data screens' JSON.
func dataFail(w http.ResponseWriter, status int, code apiCode, extra ...any) {
	w.Header().Set("Cache-Control", "no-store")
	fail(w, status, code, extra...)
}

// catalogRowView is one destination as the catalog lists it. The status is
// the best writer's, decided here with the schedule; the app only draws it.
type catalogRowView struct {
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Legacy bool   `json:"legacy"`
	// Status is on_time, late, stale, paused, continuous, unscheduled or
	// unidentified.
	Status    string `json:"status"`
	Where     string `json:"where"`
	What      string `json:"what"`
	AutoTable bool   `json:"autoTable"`
	// Writer is the first writer as a line, More how many others there are.
	Writer string `json:"writer"`
	More   int    `json:"more"`
	Queued bool   `json:"queued"`
	// LastAt and LastRows are the latest load; null where there is none or the
	// step did not say its rows.
	LastAt   *time.Time `json:"lastAt"`
	LastRows *int64     `json:"lastRows"`
	Recent   []*int64   `json:"recent"`
	// StreamHref is where a destination its manifest could not name goes: the
	// stream that writes it. Empty for every other row.
	StreamHref string `json:"streamHref"`
}

func catalogRowOf(r pages.DataRow) catalogRowView {
	where, what := catSplit(r.Target)
	v := catalogRowView{
		Target: r.Target, Kind: r.Kind, Legacy: r.Legacy, Status: string(r.Status),
		Where: where, What: what, AutoTable: strings.HasSuffix(r.Target, "/*"),
		Writer: pages.WriterLine(r), LastAt: r.LastWhen, LastRows: r.LastRows,
		Recent: append([]*int64{}, r.Recent...),
	}
	if n := len(r.Writers); n > 1 {
		v.More = n - 1
	}
	for _, w := range r.Writers {
		if w.RunInFlight != nil {
			v.Queued = true
		}
		if g := w.Gateway; g != nil && r.Target == "" && v.StreamHref == "" {
			v.StreamHref = pages.Stream{Gateway: g.Name, Path: g.Stream}.Href()
		}
	}
	return v
}

// catSplit cuts a destination into where (faint) and what (the table): the
// catalog row's own rule, see pages.splitDestination.
func catSplit(t string) (where, what string) {
	if !strings.Contains(t, "://") {
		return pages.SplitTarget(t)
	}
	if strings.HasSuffix(t, "/") {
		return t, ""
	}
	i := strings.LastIndex(t, "/")
	return t[:i+1], t[i+1:]
}

type catalogView struct {
	Now       time.Time        `json:"now"`
	Kinds     []string         `json:"kinds"`
	HasLegacy bool             `json:"hasLegacy"`
	Total     int              `json:"total"`
	Shown     int              `json:"shown"`
	Counts    map[string]int   `json:"counts"`
	Rows      []catalogRowView `json:"rows"`
}

// catalogStatusKeys are the band's choices; "all" is the count with no status
// filter, "attention" stale and late together.
var catalogStatusKeys = []string{"attention", "stale", "late", "on_time", "continuous", "paused", "unscheduled", "unidentified"}

// apiCatalog is /data's data: every destination worst first, filtered by the
// query string the page has always taken (status, kind, legacy, q).
func (u *UI) apiCatalog(w http.ResponseWriter, r *http.Request) {
	if u.catalog == nil {
		dataFail(w, http.StatusNotFound, codeNotEnabled)
		return
	}
	entries, err := u.catalog.Catalog(r.Context())
	if err != nil {
		u.log.Error("reading the catalog", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	now := time.Now()
	s := pages.BuildCatalog(entries, pages.ParseDataFilter(r.URL.Query()), now)
	v := catalogView{
		Now: now, Kinds: append([]string{}, s.Kinds...), HasLegacy: s.HasLegacy, Total: s.Total, Shown: s.Shown,
		Counts: map[string]int{"all": s.Count("")}, Rows: make([]catalogRowView, 0, len(s.Rows)),
	}
	for _, k := range catalogStatusKeys {
		v.Counts[k] = s.Count(k)
	}
	for _, row := range s.Rows {
		v.Rows = append(v.Rows, catalogRowOf(row))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}
