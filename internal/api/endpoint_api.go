package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"brevis.sh/brevis/internal/infrastructure/endpointcall"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

type paramRowView struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Note     string `json:"note"`
	Accepts  string `json:"accepts"`
	Default  string `json:"default"`
}

type originView struct {
	Main string `json:"main"`
	Sub  string `json:"sub"`
}

type lineageWriterView struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

type lineageView struct {
	Target   string             `json:"target"`
	Where    string             `json:"where"`
	What     string             `json:"what"`
	Kind     string             `json:"kind"`
	Known    bool               `json:"known"`
	Status   string             `json:"status"`
	LastAt   *time.Time         `json:"lastAt"`
	Origin   *originView        `json:"origin"`
	Writer   *lineageWriterView `json:"writer"`
	SQL      bool               `json:"sql"`
	CacheTTL string             `json:"cacheTtl"`
}

type staleView struct {
	Reason string `json:"reason"`
	Target string `json:"target"`
}

type cacheView struct {
	// State is "set" (a cache is on record), "none" (the API has no cache) or
	// "unknown" (its API was published before its settings were kept).
	State    string   `json:"state"`
	TTL      string   `json:"ttl"`
	Type     string   `json:"type"`
	AddrFrom string   `json:"addrFrom"`
	Reads    []string `json:"reads"`
	Bump     string   `json:"bump"`
}

type limitsView struct {
	Timeout string `json:"timeout"`
	MaxRows int    `json:"maxRows"`
	// MaxBytes is the byte ceiling; 0 is no ceiling.
	MaxBytes int64 `json:"maxBytes"`
}

type formFieldView struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Value    string   `json:"value"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
	MaxLen   int      `json:"maxLen"`
}

type endpointDetailView struct {
	API         string         `json:"api"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	About       string         `json:"about"`
	Keyed       bool           `json:"keyed"`
	Freshness   string         `json:"freshness"`
	PublishedAt time.Time      `json:"publishedAt"`
	Stale       *staleView     `json:"stale"`
	Lineage     []lineageView  `json:"lineage"`
	Cache       cacheView      `json:"cache"`
	Params      []paramRowView `json:"params"`
	ParamCount  int            `json:"paramCount"`
	Required    int            `json:"required"`
	Limits      limitsView     `json:"limits"`
	// CanRun says Try it is offered: a console that asks who you are and has an
	// address for the API.
	CanRun   bool            `json:"canRun"`
	Form     []formFieldView `json:"form"`
	CurlHead string          `json:"curlHead"`
	CurlBase string          `json:"curlBase"`
}

// epDuration is a Go duration in words ("6 hours"), the reader's language.
func epDuration(ctx context.Context, s string) string {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return s
	}
	switch {
	case d%time.Hour == 0:
		return i18n.N(ctx, "summary.unit.hour", int(d/time.Hour))
	case d%time.Minute == 0:
		return i18n.N(ctx, "summary.unit.minute", int(d/time.Minute))
	case d%time.Second == 0:
		return i18n.N(ctx, "summary.unit.second", int(d/time.Second))
	case d%time.Millisecond == 0:
		return i18n.N(ctx, "summary.unit.millisecond", int(d/time.Millisecond))
	}
	return s
}

// epAccepts is what a parameter accepts, in the reader's words.
func epAccepts(ctx context.Context, p pages.ParamView) string {
	switch {
	case p.Accepts != "":
		return p.Accepts
	case p.Type == "string" && p.MaxLen > 0:
		return i18n.T(ctx, "endpoint.accepts.text", "n", i18n.Number(ctx, int64(p.MaxLen)))
	case p.Type == "int" && p.Min != nil && p.Max != nil:
		return i18n.T(ctx, "endpoint.accepts.range", "min", i18n.Number(ctx, *p.Min), "max", i18n.Number(ctx, *p.Max))
	case p.Type == "int" && p.Min != nil:
		return i18n.T(ctx, "endpoint.accepts.min", "min", i18n.Number(ctx, *p.Min))
	case p.Type == "int" && p.Max != nil:
		return i18n.T(ctx, "endpoint.accepts.max", "max", i18n.Number(ctx, *p.Max))
	case p.Type == "bool":
		return i18n.T(ctx, "endpoint.accepts.bool")
	}
	return ""
}

// epOrigin is where a table's data starts, as a writer declared it.
func epOrigin(ctx context.Context, l pages.LineageRow) *originView {
	if l.Writer != nil && l.Writer.Gateway != nil {
		return &originView{Main: "POST " + l.Writer.Gateway.Stream, Sub: i18n.T(ctx, "loads.source.http")}
	}
	if l.Source.Kind == "" {
		return nil
	}
	main := l.Source.Driver
	switch l.Source.Kind {
	case "http":
		main = i18n.T(ctx, "loads.source.http")
	case "file":
		main = i18n.T(ctx, "loads.source.file")
	case "database":
		main = components.LogoTitle(ctx, l.Source.Logo)
	}
	return &originView{Main: main, Sub: l.Source.Where}
}

// loadEndpoint reads what one endpoint's page needs, as the page did: the
// endpoint, the catalog's entries with the steps' configurations attached.
func (u *UI) loadEndpoint(r *http.Request) (postgres.PublishedEndpoint, []postgres.CatalogEntry, int, apiCode) {
	e, _, status, msg := u.loadEndpointOnly(r)
	if status != 0 || u.catalog == nil {
		return e, nil, status, msg
	}
	entries, err := u.catalog.Catalog(r.Context())
	if err == nil {
		var steps map[postgres.StepKey]*postgres.WriterConfig
		if steps, err = u.catalog.StepConfigs(r.Context()); err == nil {
			pages.AttachStepConfigs(entries, steps)
		}
	}
	if err != nil {
		u.log.Error("reading the catalog for an endpoint", "error", err)
		return e, nil, http.StatusInternalServerError, codeUnavailable
	}
	return e, entries, 0, ""
}

// canRun is whether this console may run the endpoint: it asks who you are,
// and it has an address for the API.
func (u *UI) canRun(api string) bool {
	return u.callsProtected && u.calls != nil && u.calls.Configured(api)
}

// apiEndpoint is one endpoint's page: its contract, whether the data it serves
// is stale and why, where the data comes from and how answers are cached.
func (u *UI) apiEndpoint(w http.ResponseWriter, r *http.Request) {
	e, entries, status, msg := u.loadEndpoint(r)
	if status != 0 {
		dataFail(w, status, msg)
		return
	}
	ctx := i18n.WithLang(r.Context(), i18n.Negotiate(r))
	v := pages.BuildEndpoint(e, entries, time.Now())
	out := endpointDetailView{
		API: v.Row.API, Name: v.Row.Name, Path: v.Row.Path, About: v.About, Keyed: v.Row.Keyed,
		Freshness: v.Row.Freshness, PublishedAt: v.Row.PublishedAt,
		Lineage: make([]lineageView, 0, len(v.Lineage)), Params: make([]paramRowView, 0, len(v.Params)),
		ParamCount: len(v.Row.Params), Required: len(v.Row.Required), Form: []formFieldView{},
		Limits:   limitsView{Timeout: epDuration(ctx, v.Limits.Timeout), MaxRows: v.Limits.MaxRows, MaxBytes: v.Limits.MaxBytesBilled},
		CurlHead: pages.CurlHead, CurlBase: v.CurlBase(), CanRun: u.canRun(e.API),
	}
	if l := v.StaleLine(); l != nil {
		s := &staleView{Target: l.Target}
		if l.Writer != nil {
			s.Reason = l.Writer.Reason(ctx)
		}
		out.Stale = s
	}
	sql := u.preview.Configured()
	for _, l := range v.Lineage {
		where, what := catSplit(l.Target)
		lv := lineageView{
			Target: l.Target, Where: where, What: what, Known: l.Known, Status: string(l.Status), LastAt: l.LastWhen,
			Origin: epOrigin(ctx, l), SQL: sql,
		}
		if scheme, _, ok := strings.Cut(l.Target, "://"); ok {
			lv.Kind = scheme
		}
		if !l.Known {
			lv.Status = ""
		}
		if l.Writer != nil {
			lv.Writer = &lineageWriterView{Name: catWriterName(l.Writer.CatalogWriter), Href: l.WriterHref()}
		}
		if c := v.Manifest.Cache; c != nil {
			lv.CacheTTL = epDuration(ctx, c.TTL)
		}
		out.Lineage = append(out.Lineage, lv)
	}
	switch c := v.Manifest.Cache; {
	case c != nil:
		out.Cache = cacheView{State: "set", TTL: epDuration(ctx, c.TTL), Type: c.Type, AddrFrom: c.AddrFrom,
			Reads: append([]string{}, v.Row.Reads...), Bump: v.BumpCommand()}
		if c.Type == "redis" {
			out.Cache.Type = "Redis"
		}
	case v.Manifest.Connection == "":
		out.Cache = cacheView{State: "unknown", Reads: []string{}}
	default:
		out.Cache = cacheView{State: "none", Reads: []string{}}
	}
	for _, p := range v.Params {
		out.Params = append(out.Params, paramRowView{Name: p.Name, Type: p.Type, Required: p.Required, Note: p.Note,
			Accepts: epAccepts(ctx, p), Default: p.Default})
	}
	if out.CanRun {
		for _, f := range pages.TryForm(e.Endpoint.Params, nil) {
			out.Form = append(out.Form, formFieldView{Name: f.Name, Kind: f.Kind, Value: f.Value,
				Options: append([]string{}, f.Options...), Required: f.Required, MaxLen: f.MaxLen})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type callBody struct {
	Params map[string]string `json:"params"`
}

type answerView struct {
	Status     int        `json:"status"`
	StatusText string     `json:"statusText"`
	Cache      string     `json:"cache"`
	Millis     int64      `json:"millis"`
	Received   int        `json:"received"`
	Truncated  bool       `json:"truncated"`
	Columns    []string   `json:"columns"`
	Rows       [][]string `json:"rows"`
	// Error is the API's own words for a refusal.
	Error string `json:"error"`
}

// epRunFailure is why a call brought nothing back, as a code: the console's own failures by kind, anything
// else as codeRunFailed with the error's own text (the second result) for the screen to show as it came.
func epRunFailure(err error) (apiCode, string) {
	for known, code := range map[error]apiCode{
		endpointcall.ErrNotConfigured: codeEndpointNotConfigured,
		endpointcall.ErrUnbuilt:       codeEndpointUnbuilt,
		endpointcall.ErrUnreachable:   codeEndpointUnreachable,
		endpointcall.ErrUnreadable:    codeEndpointUnreadable,
		endpointcall.ErrTooLarge:      codeEndpointTooLarge,
		endpointcall.ErrNotAnswer:     codeEndpointNotAnswer,
	} {
		if errors.Is(err, known) {
			return code, ""
		}
	}
	return codeRunFailed, err.Error()
}

// apiEndpointCall is Try it: a POST with the parameters as JSON, so a call --
// a warehouse query -- is never a link a crawler follows. The same refusals as
// the page: only a console that asks who you are, only an API it has an
// address for, only the declared parameters, empty ones left to the API's
// defaults. An answer that is a refusal from the API is a 200 with its status
// inside; the console's own failures are errors.
func (u *UI) apiEndpointCall(w http.ResponseWriter, r *http.Request) {
	// JSON only: a cross-site form cannot send this content type unasked.
	if mt, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(mt) != "application/json" {
		dataFail(w, http.StatusUnsupportedMediaType, codeInvalidRequest)
		return
	}
	var body callBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest)
		return
	}
	e, _, status, msg := u.loadEndpointOnly(r)
	if status != 0 {
		dataFail(w, status, msg)
		return
	}
	if !u.canRun(e.API) {
		dataFail(w, http.StatusForbidden, codeForbidden)
		return
	}
	q := url.Values{}
	for k, v := range body.Params {
		q.Set(k, v)
	}
	a, err := u.calls.Call(r.Context(), e.API, e.Path, pages.TryValues(e.Endpoint.Params, q))
	if err != nil {
		if code, detail := epRunFailure(err); detail != "" {
			dataFail(w, http.StatusBadGateway, code, "detail", detail)
		} else {
			dataFail(w, http.StatusBadGateway, code)
		}
		return
	}
	out := answerView{Status: a.Status, StatusText: http.StatusText(a.Status), Cache: a.Cache, Millis: a.Millis,
		Received: a.Received, Truncated: a.Truncated, Error: a.Error, Columns: append([]string{}, a.Columns...),
		Rows: make([][]string, 0, len(a.Rows))}
	for _, row := range a.Rows {
		cells := make([]string, 0, len(row))
		for _, c := range row {
			cells = append(cells, pages.Cell(c))
		}
		out.Rows = append(out.Rows, cells)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// loadEndpointOnly is the endpoint without the catalog: a call needs its
// contract, not the lineage.
func (u *UI) loadEndpointOnly(r *http.Request) (postgres.PublishedEndpoint, []postgres.CatalogEntry, int, apiCode) {
	if u.endpoints == nil {
		return postgres.PublishedEndpoint{}, nil, http.StatusNotFound, codeNotEnabled
	}
	e, found, err := u.endpoints.Get(r.Context(), r.PathValue("api"), r.PathValue("name"))
	switch {
	case err != nil:
		u.log.Error("reading an endpoint", "error", err)
		return e, nil, http.StatusInternalServerError, codeUnavailable
	case !found:
		return e, nil, http.StatusNotFound, codeNotFound
	}
	return e, nil, 0, ""
}
