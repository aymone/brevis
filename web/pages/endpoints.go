package pages

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/endpointcall"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// EndpointsView is Data → Endpoints (screen-endpoints): every published
// brevis-sql-api path the filter keeps, worst first by the data it reads,
// and the counts of every endpoint whatever is shown.
type EndpointsView struct {
	Rows []EndpointRow

	// Total, APIs, Fresh and Stale count every endpoint; Caches and
	// Connections are what the APIs published -- "redis", "warehouse
	// (bigquery)" -- for the summary.
	Total, APIs, Fresh, Stale int
	Caches, Connections       []string
	// APINames are the APIs present, sorted: the API filter's choices.
	APINames []string

	Filter EndpointFilter
	Now    time.Time
}

// EndpointFilter narrows the list: Q searches the path, the API, the tables
// read and the parameters.
type EndpointFilter struct {
	// Q searches the path, the API, the parameters and the tables read; API
	// is one API's name; State is "fresh" or "stale". "" is all.
	Q, API, State string
}

// ParseEndpointFilter reads the filter from a query string; a state it does
// not know is ignored rather than turned into an empty page.
func ParseEndpointFilter(q url.Values) EndpointFilter {
	f := EndpointFilter{Q: strings.TrimSpace(q.Get("q")), API: strings.TrimSpace(q.Get("api"))}
	if r := []rune(f.Q); len(r) > searchCeiling {
		f.Q = string(r[:searchCeiling])
	}
	if s := q.Get("state"); s == "fresh" || s == "stale" {
		f.State = s
	}
	return f
}

// EndpointRow is one published endpoint.
type EndpointRow struct {
	API, Name, Path string
	// Keyed is true when each request reads as its key's tenant.
	Keyed       bool
	Params      []string
	Reads       []string
	PublishedAt time.Time
	// Required names the parameters a request must carry.
	Required map[string]bool

	// About is the first line of the note above the endpoint in its file.
	About string

	// Freshness is the worst of the tables it reads, by the catalog: "stale"
	// (late or stale), "fresh", or "" when the catalog knows none of them.
	Freshness string

	// Read is the catalog's row of the first table it reads, when the
	// catalog knows it: when it was rebuilt, and by whom.
	Read *DataRow

	// TTL is how long an answer is cached, from the API's manifest; "" with
	// no cache on record.
	TTL string
}

// SearchText is what a search looks in, lower-cased: the path, the API, the
// parameters and the tables it reads.
func (r EndpointRow) SearchText() string {
	parts := append([]string{r.Path, r.API, r.Name}, r.Params...)
	parts = append(parts, r.Reads...)
	return strings.ToLower(strings.Join(parts, "\n"))
}

// Href is the endpoint's own page.
func (r EndpointRow) Href() string {
	return "/data/endpoints/" + url.PathEscape(r.API) + "/" + url.PathEscape(r.Name)
}

// TargetHref is a read target's page: /data/target, which forwards it to the
// stream or the load that owns it now that Catalog is Streams and Loads.
func TargetHref(target string) string { return "/data/target?u=" + url.QueryEscape(target) }

func rowOf(e postgres.PublishedEndpoint) EndpointRow {
	r := EndpointRow{API: e.API, Name: e.Name, Path: e.Path, Keyed: e.Tenant == "key",
		Reads: e.Reads, PublishedAt: e.PublishedAt, About: NoteHead(e.Endpoint.Notes[""])}
	if c := e.Manifest.Cache; c != nil {
		r.TTL = c.TTL
	}
	for _, p := range e.Endpoint.Params {
		r.Params = append(r.Params, p.Name)
		if p.Required {
			if r.Required == nil {
				r.Required = map[string]bool{}
			}
			r.Required[p.Name] = true
		}
	}
	return r
}

// freshnessRank orders the list: stale first, then fresh, then unknown.
var freshnessRank = map[string]int{"stale": 0, "fresh": 1, "": 2}

// BuildEndpoints is every endpoint under the filter (screen-endpoints T2),
// judged by the catalog's view of the tables it reads -- the worst of them
// -- worst first, then by API and path. The search drops what it does not
// match, on the server, so a searched list is a link.
func BuildEndpoints(all []postgres.PublishedEndpoint, entries []postgres.CatalogEntry, f EndpointFilter, now time.Time) EndpointsView {
	v := EndpointsView{Filter: f, Now: now}
	byTarget := judgedTargets(entries, now)
	apis := map[string]bool{}
	q := strings.ToLower(strings.TrimSpace(f.Q))
	if r := []rune(q); len(r) > searchCeiling {
		q = string(r[:searchCeiling])
	}
	for _, e := range all {
		r := judgedRow(e, byTarget)
		v.Total++
		apis[e.API] = true
		switch r.Freshness {
		case "stale":
			v.Stale++
		case "fresh":
			v.Fresh++
		}
		if c := e.Manifest.Cache; c != nil && !containsString(v.Caches, c.Type) {
			v.Caches = append(v.Caches, c.Type)
		}
		if m := e.Manifest; m.Connection != "" {
			if conn := m.Connection + " (" + m.Dialect + ")"; !containsString(v.Connections, conn) {
				v.Connections = append(v.Connections, conn)
			}
		}
		switch {
		case f.API != "" && e.API != f.API,
			f.State != "" && r.Freshness != f.State,
			q != "" && !strings.Contains(r.SearchText(), q):
			continue
		}
		v.Rows = append(v.Rows, r)
	}
	v.APIs = len(apis)
	for api := range apis {
		v.APINames = append(v.APINames, api)
	}
	sort.Strings(v.APINames)
	sort.SliceStable(v.Rows, func(i, j int) bool {
		a, b := v.Rows[i], v.Rows[j]
		if freshnessRank[a.Freshness] != freshnessRank[b.Freshness] {
			return freshnessRank[a.Freshness] < freshnessRank[b.Freshness]
		}
		if a.API != b.API {
			return a.API < b.API
		}
		return a.Path < b.Path
	})
	return v
}

// judgedTargets is the catalog's rows by destination, each judged as the
// Catalog judges it.
func judgedTargets(entries []postgres.CatalogEntry, now time.Time) map[string]DataRow {
	byTarget := map[string]DataRow{}
	for _, row := range BuildData(entries, now).Rows {
		byTarget[row.Target] = row
	}
	return byTarget
}

// judgedRow is an endpoint's row with how its data stands: "stale" when a
// table it reads is late or stale, "fresh" when the catalog knows one and
// none is, "" when it knows none. Read is the first table's row.
func judgedRow(e postgres.PublishedEndpoint, byTarget map[string]DataRow) EndpointRow {
	r := rowOf(e)
	for i, target := range e.Reads {
		row, known := byTarget[target]
		if !known {
			continue
		}
		if i == 0 {
			read := row
			r.Read = &read
		}
		switch {
		case staleStatus(row.Status):
			r.Freshness = "stale"
		case r.Freshness == "":
			r.Freshness = "fresh"
		}
	}
	return r
}

// staleStatus is a catalog status an endpoint calls stale data.
func staleStatus(s catalog.Status) bool { return s == catalog.Late || s == catalog.Stale }

// EndpointView is one endpoint's page: its contract, as published, and how
// the data it reads stands (screen-endpoints T4).
type EndpointView struct {
	Row    EndpointRow
	Params []ParamView
	Limits catalog.EndpointLimits
	// Lineage is one line per table read, in the endpoint's order; Manifest
	// what its API published beside it; Now the page's clock.
	Lineage  []LineageRow
	Manifest postgres.APIManifest
	Now      time.Time
	// About is the note above the endpoint in its file, if its author marked
	// one.
	About string

	// CanRun says this console may run the endpoint: it asks who you are,
	// and it has an address for the API. Without both there is no Try it.
	CanRun bool
	// Form is Try it's fields; Answer what the last Run brought back, or
	// RunErr why there is none.
	Form   []FormField
	Answer *endpointcall.Answer
	RunErr error
}

// ParamView is one parameter as a caller reads it. Accepts is a list the
// machine writes -- the values, the periods, a date's shape -- and the rest
// is said in the reader's words from MaxLen, Min and Max.
type ParamView struct {
	Name, Type, Accepts, Default, Note string
	Required                           bool
	MaxLen                             int
	Min, Max                           *int64
}

// LineageRow is one table an endpoint reads, and what is known upstream of
// it: whether the catalog knows it, how it stands, its best writer -- the one
// the table's status is -- and where that writer reads, as declared.
type LineageRow struct {
	Target   string
	Known    bool
	Status   catalog.Status
	LastWhen *time.Time
	Writer   *WriterView
	Source   Source
}

// Stale says the table is late or stale.
func (l LineageRow) Stale() bool { return l.Known && staleStatus(l.Status) }

// WriterHref is the writer's page: a stream's, or a load's.
func (l LineageRow) WriterHref() string {
	if l.Writer == nil {
		return ""
	}
	if g := l.Writer.Gateway; g != nil {
		return Stream{Gateway: g.Name, Path: g.Stream}.Href()
	}
	return LoadTarget{Workflow: l.Writer.Workflow, Step: l.Writer.Node, URI: l.Target}.Href()
}

// BuildEndpoint is the page of one endpoint, judged by the catalog's entries
// (with the writers' configurations attached, for where they read).
func BuildEndpoint(e postgres.PublishedEndpoint, entries []postgres.CatalogEntry, now time.Time) EndpointView {
	byTarget := judgedTargets(entries, now)
	v := EndpointView{Row: judgedRow(e, byTarget), Limits: e.Endpoint.Limits, About: e.Endpoint.Notes[""],
		Manifest: e.Manifest, Now: now}
	for _, target := range e.Reads {
		l := LineageRow{Target: target}
		if row, known := byTarget[target]; known {
			l.Known, l.Status, l.LastWhen = true, row.Status, row.LastWhen
			for i := range row.Writers {
				if row.Writers[i].Verdict.Status == row.Status {
					l.Writer = &row.Writers[i]
					break
				}
			}
			if l.Writer == nil && len(row.Writers) > 0 {
				l.Writer = &row.Writers[0]
			}
			if l.Writer != nil && l.Writer.Gateway == nil {
				l.Source = SourceOf(l.Writer.Config)
			}
		}
		v.Lineage = append(v.Lineage, l)
	}
	for _, p := range e.Endpoint.Params {
		pv := ParamView{Name: p.Name, Type: p.Type, Required: p.Required, Note: e.Endpoint.Notes["params."+p.Name],
			MaxLen: p.MaxLen, Min: p.Min, Max: p.Max}
		if p.Default != nil {
			pv.Default = *p.Default
		}
		switch {
		case len(p.Values) > 0:
			pv.Accepts = strings.Join(p.Values, ", ")
		case p.Type == "date":
			pv.Accepts = "YYYY-MM-DD"
		case p.Type == "period":
			pv.Accepts = strings.Join(periods, ", ")
		}
		v.Params = append(v.Params, pv)
	}
	return v
}

// StaleLine is the first table read that is late or stale: the one the
// warning names.
func (v EndpointView) StaleLine() *LineageRow {
	for i := range v.Lineage {
		if v.Lineage[i].Stale() {
			return &v.Lineage[i]
		}
	}
	return nil
}

// BumpCommand is Clear cache (D5): the command that clears what the cache
// holds for the tables this endpoint reads, built from what its API
// published -- the variable the cache's address is read from and the dialect
// -- with each table named as a build keys it: no project on BigQuery,
// lowered on Postgres. A table of another warehouse is not the cache's. ""
// with no cache on record, or no table it keys.
func (v EndpointView) BumpCommand() string {
	c := v.Manifest.Cache
	if c == nil || c.AddrFrom == "" {
		return ""
	}
	var names []string
	for _, target := range v.Row.Reads {
		scheme, rest, ok := strings.Cut(target, "://")
		if !ok || scheme != v.Manifest.Dialect {
			continue
		}
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) != 3 {
			continue
		}
		name := parts[1] + "." + parts[2]
		if scheme == "postgres" {
			name = strings.ToLower(name)
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "brevis-sql generation bump --generations-from " + c.AddrFrom + " --dialect " + v.Manifest.Dialect + " " + strings.Join(names, " ")
}

// CurlHead is the curl's fixed start. The key is a placeholder, always: the
// console's own is never written into a page.
const CurlHead = "curl -H 'Authorization: Bearer <key>' "

// CurlBase is the address the curl calls, before its query string. ui.js
// rebuilds the rest from the form as somebody types, with the same escaping
// url.QueryEscape uses here, so a quote typed into a field cannot end the
// shell's quoted string.
func (v EndpointView) CurlBase() string {
	return "<api address>" + v.Row.Path
}

// EndpointRef is an endpoint on a model's page, as one of its readers.
type EndpointRef struct {
	API, Name, Path, Href string
}

// ReadersOf are the endpoints over a target, for its page.
func ReadersOf(all []postgres.PublishedEndpoint) []EndpointRef {
	var out []EndpointRef
	for _, e := range all {
		r := rowOf(e)
		out = append(out, EndpointRef{API: r.API, Name: r.Name, Path: r.Path, Href: r.Href()})
	}
	return out
}

// FormField is one input of Try it, typed as its parameter is.
type FormField struct {
	Name, Kind, Value string
	// Options are a select's: an enum's values, a bool's, a period's. Empty
	// for an input.
	Options  []string
	Required bool
	MaxLen   int
}

// periods are what a period parameter accepts, in brevis-sql-api's order.
//
// A LIST THIS CANNOT IMPORT, SO IT SAYS WHERE IT CAME FROM:
// `sql/internal/api/period.go`, `periods` -- the engine imports nothing of
// the sql module. If that one moves, this one moves.
var periods = []string{"today", "yesterday", "last_7_days", "last_30_days", "last_week", "last_month", "month_to_date"}

// TryForm is the endpoint's parameters as a form, holding what was last sent
// -- or, before anything was, each parameter's default.
func TryForm(params []catalog.EndpointParam, sent url.Values) []FormField {
	var out []FormField
	for _, p := range params {
		f := FormField{Name: p.Name, Required: p.Required, MaxLen: p.MaxLen}
		switch p.Type {
		case "enum":
			f.Kind, f.Options = "select", p.Values
		case "bool":
			f.Kind, f.Options = "select", []string{"true", "false"}
		case "period":
			f.Kind, f.Options = "select", periods
		case "date":
			f.Kind = "date"
		case "int":
			f.Kind = "number"
		default:
			f.Kind = "text"
		}
		if v, there := sent[p.Name]; there {
			f.Value = v[0]
		} else if p.Default != nil && sent == nil {
			f.Value = *p.Default
		}
		out = append(out, f)
	}
	return out
}

// TryValues are the declared parameters a submitted form carries, the empty
// ones left out: an empty select is "use the default", which is the API's
// to apply, and an undeclared name is not forwarded at all.
func TryValues(params []catalog.EndpointParam, q url.Values) url.Values {
	out := url.Values{}
	for _, p := range params {
		if v := strings.TrimSpace(q.Get(p.Name)); v != "" {
			out.Set(p.Name, v)
		}
	}
	return out
}
