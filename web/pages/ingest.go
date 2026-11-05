package pages

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"brevis.sh/brevis/web/i18n"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
)

// Ingest is what Catalog was, read as the two mechanisms it mixed: gateway
// STREAMS, which ingest continuously, and SDK LOADS, which a workflow step
// lands on its schedule. Both come from the same catalog read; this file only
// regroups it.

// GatewayGroup is one gateway and its streams.
type GatewayGroup struct {
	Name    string
	Streams []Stream
}

// Stream is a gateway stream, the unit of continuous ingest: where events are
// posted, the one destination they land on, and the dead letter that destination's
// refusals go to. THE DEAD LETTER BELONGS TO THE STREAM -- it is never an item
// of its own, which is what Catalog got wrong.
type Stream struct {
	Gateway, Path string
	Sink          Place
	DeadLetter    *Place
	PublishedAt   time.Time

	// Config is the stream as last published; nil for a version-1 manifest,
	// which described nothing but where it lands.
	Config *postgres.WriterConfig
}

// Place is a destination: its URI and its kind (the scheme). Note is why a
// gateway destination has no URI -- its manifest could not name it -- and is
// empty otherwise.
type Place struct {
	URI, Kind, Note string
}

// Label is the URI, or "unidentified" for a destination nothing could name.
func (p Place) Label() string {
	if p.URI == "" {
		return "unidentified"
	}
	return p.URI
}

// Field is a declared configuration value, as text; "" when the stream does
// not declare it, which the screens show as "—".
func (s Stream) Field(path string) string {
	if s.Config == nil {
		return ""
	}
	for _, f := range s.Config.Fields {
		if f.Path == path && f.Value != nil {
			return valueText(f.Value)
		}
	}
	return ""
}

// Mode is how the sink writes, as declared: `sink.write`, or the write of an
// auto_table's tables.
func (s Stream) Mode() string {
	if s.Field("sink.type") == "auto_table" {
		return s.Field("sink.into.write")
	}
	return s.Field("sink.write")
}

// Merge says whether the sink merges, which there is no field for: it is a
// write mode. Nil when the stream does not declare a mode -- "no" would be a
// guess.
func (s Stream) Merge() *bool {
	m := s.Mode()
	if m == "" {
		return nil
	}
	merge := m == "merge"
	return &merge
}

// Href is the stream's page: the gateway, then the stream's path as the tail.
func (s Stream) Href() string {
	return "/ingest/streams/" + url.PathEscape(s.Gateway) + "/" + strings.TrimPrefix(s.Path, "/")
}

// StreamsOf regroups the catalog's gateway writers into gateways and their
// streams, both by name. A stream is keyed by (gateway, path); its sink and
// dead letter are the entries its writer lands on in each role.
func StreamsOf(entries []postgres.CatalogEntry) []GatewayGroup {
	type key struct{ gateway, path string }
	streams := map[key]*Stream{}
	var order []key
	for _, e := range entries {
		for _, w := range e.Writers {
			g := w.Gateway
			if g == nil {
				continue
			}
			k := key{g.Name, g.Stream}
			s := streams[k]
			if s == nil {
				s = &Stream{Gateway: g.Name, Path: g.Stream}
				streams[k] = s
				order = append(order, k)
			}
			if g.PublishedAt.After(s.PublishedAt) {
				s.PublishedAt = g.PublishedAt
			}
			place := Place{URI: e.Target, Kind: e.Kind}
			if e.Target == "" {
				place.Note = g.Note
			}
			switch g.Role {
			case "dead_letter":
				s.DeadLetter = &place
				if s.Config == nil {
					s.Config = g.Config
				}
			default:
				s.Sink = place
				if g.Config != nil {
					s.Config = g.Config
				}
			}
		}
	}

	byGateway := map[string]*GatewayGroup{}
	var names []string
	for _, k := range order {
		g := byGateway[k.gateway]
		if g == nil {
			g = &GatewayGroup{Name: k.gateway}
			byGateway[k.gateway] = g
			names = append(names, k.gateway)
		}
		g.Streams = append(g.Streams, *streams[k])
	}
	sort.Strings(names)
	out := make([]GatewayGroup, 0, len(names))
	for _, n := range names {
		g := byGateway[n]
		sort.SliceStable(g.Streams, func(i, j int) bool { return g.Streams[i].Path < g.Streams[j].Path })
		out = append(out, *g)
	}
	return out
}

// LoadTarget is one step landing on one destination, judged against that
// step's own schedule.
type LoadTarget struct {
	Workflow, Step string
	Place
	Legacy     bool
	Status     catalog.Status
	LastLoaded time.Time
	Rows       *int64

	// Cron and Timezone are the step's schedule; HasSchedule is false for
	// a workflow that only runs by hand.
	Cron, Timezone string
	HasSchedule    bool

	// Source is where the step reads, as its configuration declares it;
	// zero when it declared none.
	Source Source

	// Recent is the rows of the destination's last loads, oldest first, at
	// most recentShown -- only when this step is its only writer: an entry's
	// history is shared, and another step's loads in this step's sparkline
	// would be a lie.
	Recent []*int64

	// Many is set when the step writes more than one destination, so its
	// link has to say which.
	Many bool

	// RunInFlight is a run of the workflow that is queued or running: on a
	// late load, the run about to fix it.
	RunInFlight *string
}

// Fixing is the run in flight when the load is late or stale -- the one
// worth a link -- and nil otherwise.
func (t LoadTarget) Fixing() *string {
	if t.Status == catalog.Late || t.Status == catalog.Stale {
		return t.RunInFlight
	}
	return nil
}

// Href is the load's page; the destination goes in the query only when the
// step writes more than one.
func (t LoadTarget) Href() string {
	h := "/ingest/loads/" + url.PathEscape(t.Workflow) + "/" + url.PathEscape(t.Step)
	if t.Many {
		h += "?target=" + url.QueryEscape(t.URI)
	}
	return h
}

// recentShown is how many of a destination's last loads a row draws: the
// handoff's "last 12".
const recentShown = 12

// LoadsOf is every step landing on every destination, one row each, in the
// Catalog's order (screen-loads T2): worst first, then the most recently
// loaded. A gateway's stream is not a load. `now` judges freshness.
func LoadsOf(entries []postgres.CatalogEntry, now time.Time) []LoadTarget {
	var out []LoadTarget
	written := map[[2]string]int{}
	for _, e := range entries {
		steps := 0
		for _, w := range e.Writers {
			if w.Gateway == nil {
				steps++
			}
		}
		for _, w := range e.Writers {
			if w.Gateway != nil {
				continue
			}
			t := LoadTarget{
				Workflow: w.Workflow, Step: w.Node, Place: Place{URI: e.Target, Kind: e.Kind},
				Legacy: e.Legacy, Status: verdictOf(e, w, now).Status,
				LastLoaded: w.LastLoaded, Rows: w.LastRows, RunInFlight: w.RunInFlight,
				Cron: w.Cron, Timezone: w.Timezone, HasSchedule: w.HasSchedule,
				Source: SourceOf(w.Config),
			}
			if steps == 1 {
				t.Recent = e.Recent[max(0, len(e.Recent)-recentShown):]
			}
			out = append(out, t)
			written[[2]string{w.Workflow, w.Node}]++
		}
	}
	for i := range out {
		out[i].Many = written[[2]string{out[i].Workflow, out[i].Step}] > 1
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if worstFirst[a.Status] != worstFirst[b.Status] {
			return worstFirst[a.Status] < worstFirst[b.Status]
		}
		if !a.LastLoaded.Equal(b.LastLoaded) {
			return a.LastLoaded.After(b.LastLoaded)
		}
		if a.Workflow != b.Workflow {
			return a.Workflow < b.Workflow
		}
		if a.Step != b.Step {
			return a.Step < b.Step
		}
		return a.URI < b.URI
	})
	return out
}

// LoadsView is /ingest/loads as the template draws it (screen-loads): the
// loads the filter keeps, and the counts beside them.
type LoadsView struct {
	Rows []LoadTarget

	// Summary, Total and Workflows describe every load the page holds --
	// legacy ones only when asked for -- whatever else is filtered: the line
	// under the title is the page's, not the filter's.
	Summary          map[catalog.Status]int
	Total, Workflows int

	// counts is each band choice's count, with the origin, the search and
	// the legacy switch as they are and the status left out, so a count
	// cannot disagree with the list its link opens.
	counts map[string]int

	// Sources are the origins present, in sourceOrder, for the origin
	// filter; "none" is a step that declared none.
	Sources   []string
	HasLegacy bool
	Filter    DataFilter
	Now       time.Time

	// Drawer is the load the URL names (/ingest/loads/{workflow}/{step}),
	// drawn open over the list (screen-loads T4); nil draws none. Missing is
	// a step it named that writes no destination Brevis knows.
	Drawer  *LoadView
	Missing string
}

// sourceOrder is the origin filter's order: the SDK's own kinds, somebody
// else's driver, then none declared.
var sourceOrder = []string{"http", "file", "database", "other", "none"}

// sourceKey is a load's origin as the filter names it.
func sourceKey(s Source) string {
	if s.Kind == "" {
		return "none"
	}
	return s.Kind
}

// SearchText is what a search looks in, lower-cased: the workflow, the step,
// where the step reads and the destination.
func (t LoadTarget) SearchText() string {
	return strings.ToLower(strings.Join([]string{t.Workflow, t.Step, t.Source.Driver, t.Source.Where, t.URI}, "\n"))
}

// BuildLoads is every load under the filter (screen-loads T3). A legacy row
// waits until it is asked for, as in the Catalog; the search is the
// server's, so a searched list is a link and works with no script.
func BuildLoads(entries []postgres.CatalogEntry, f DataFilter, now time.Time) LoadsView {
	v := LoadsView{Summary: map[catalog.Status]int{}, counts: map[string]int{}, Filter: f, Now: now}
	workflows := map[string]bool{}
	present := map[string]bool{}
	q := strings.ToLower(f.Q)
	for _, t := range LoadsOf(entries, now) {
		v.HasLegacy = v.HasLegacy || t.Legacy
		if t.Legacy && !f.Legacy {
			continue
		}
		v.Summary[t.Status]++
		v.Total++
		workflows[t.Workflow] = true
		present[sourceKey(t.Source)] = true

		if f.Source != "" && sourceKey(t.Source) != f.Source || q != "" && !strings.Contains(t.SearchText(), q) {
			continue
		}
		v.counts[""]++
		v.counts[string(t.Status)]++
		if t.Status == catalog.Late || t.Status == catalog.Stale {
			v.counts["attention"]++
		}
		if f.keepsStatus(t.Status) {
			v.Rows = append(v.Rows, t)
		}
	}
	v.Workflows = len(workflows)
	for _, s := range sourceOrder {
		if present[s] {
			v.Sources = append(v.Sources, s)
		}
	}
	return v
}

// loadSchedule is when a step runs, as its row says it: in words where they
// are sure, the expression otherwise, "by hand" with none.
func loadSchedule(t LoadTarget) i18n.Cat {
	if !t.HasSchedule {
		return i18n.Cat{i18n.M("loads.manual")}
	}
	return cronWords(t.Cron, t.Timezone)
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// LoadView is one load's drawer (screen-loads T4): the destination's view
// (TargetView) narrowed to the one step, plus who that step is and where it
// reads.
type LoadView struct {
	TargetView
	Workflow, Step string

	// Read is how many rows the newest load's step read (RowsRead), nil
	// when its run's stages do not say.
	Read *int64

	// Source is where the step reads, as it declared.
	Source Source

	// Choices are the destinations a step writing several writes, the open
	// one marked; with none open (no ?target=) the drawer asks which.
	Choices []LoadChoice

	// Run is what Run now does (screen-loads T6): "now" runs a workflow with
	// no parameters at once, "params" opens its Run drawer to ask them, and
	// "" offers nothing -- a console with no scheduler, or a workflow it
	// cannot read. Back is the drawer's own address, which the run comes
	// back to.
	Run, Back string
}

// Choosing is a drawer that asks which destination, having none open.
func (v LoadView) Choosing() bool { return v.Row.Target == "" && len(v.Choices) > 0 }

// BuildLoad narrows a destination to the loads and the writer of one step:
// another workflow writing the same destination is a different load, and
// its rows in this step's bars would be a lie.
func BuildLoad(d postgres.TargetDetail, workflow, step string, now time.Time) LoadView {
	var writers []postgres.CatalogWriter
	for _, w := range d.Writers {
		if w.Gateway == nil && w.Workflow == workflow && w.Node == step {
			writers = append(writers, w)
		}
	}
	var loads []postgres.TargetLoad
	for _, l := range d.Loads {
		if l.Workflow == workflow && l.Node == step {
			loads = append(loads, l)
		}
	}
	d.Writers, d.Loads = writers, loads
	if len(d.Writers) > 1 || len(d.Writers) == 0 {
		d.Recent = nil
	}
	return LoadView{TargetView: BuildTarget(d, now), Workflow: workflow, Step: step}
}

// Writer is the step's own writer row, with its verdict.
func (v LoadView) Writer() *WriterView {
	if len(v.Row.Writers) == 0 {
		return nil
	}
	return &v.Row.Writers[0]
}

// loadsShown is how many loads the destination read returns (its LIMIT).
const loadsShown = 30

// LoadChoice is one destination a step writes, in the drawer that asks
// which; Current is the one open.
type LoadChoice struct {
	URI, Href string
	Current   bool
}

// AttachStreamConfigs gives each gateway writer its stream's configuration,
// from the one read of every stream's (the catalog read leaves them out).
func AttachStreamConfigs(entries []postgres.CatalogEntry, configs map[postgres.StreamKey]*postgres.WriterConfig) {
	for _, e := range entries {
		for _, w := range e.Writers {
			if g := w.Gateway; g != nil && g.Config == nil {
				g.Config = configs[postgres.StreamKey{Gateway: g.Name, Path: g.Stream}]
			}
		}
	}
}

// StreamFilter narrows /ingest/streams: a search over the path, the sink, the
// dead letter and the gateway, and the sink's kind. In the URL, like every
// list's.
type StreamFilter struct {
	Q, Kind string
}

// ParseStreamFilter reads the filter from a query string; a kind it does not
// know is ignored rather than turned into an empty page.
func ParseStreamFilter(q url.Values) StreamFilter {
	f := StreamFilter{Q: strings.TrimSpace(q.Get("q"))}
	if r := []rune(f.Q); len(r) > searchCeiling {
		f.Q = string(r[:searchCeiling])
	}
	if k := q.Get("kind"); filterKinds[k] {
		f.Kind = k
	}
	return f
}

// StreamsView is /ingest/streams as drawn (screen-streams): the streams the
// filter keeps, one row each, and the counts of every stream whatever is
// shown.
type StreamsView struct {
	Rows                            []Stream
	Total, Gateways, WithDeadLetter int
	// HealthCounts is every stream's health state, whatever the filter shows.
	HealthCounts map[string]int
	// Healths is each stream's health by Stream.Key, the live one when the
	// builder was given a source.
	Healths map[string]StreamHealth
	Kinds   []string
	Filter  StreamFilter
}

// SearchText is what a search looks in, lower-cased: the gateway, the path,
// where it lands and where its refusals go.
func (s Stream) SearchText() string {
	parts := []string{s.Gateway, s.Path, s.Sink.URI}
	if s.DeadLetter != nil {
		parts = append(parts, s.DeadLetter.URI)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

// Key is the stream's identity as one string, for the page's markup.
func (s Stream) Key() string { return s.Gateway + " " + s.Path }

// BuildStreamsWith is BuildStreams with the health of each stream said by
// health, which is how the live metrics reach the list and its counts.
func BuildStreamsWith(entries []postgres.CatalogEntry, f StreamFilter, health func(Stream) StreamHealth) StreamsView {
	v := StreamsView{Filter: f, HealthCounts: map[string]int{}, Healths: map[string]StreamHealth{}}
	q := strings.ToLower(f.Q)
	for _, g := range StreamsOf(entries) {
		v.Gateways++
		for _, s := range g.Streams {
			v.Total++
			h := health(s)
			v.HealthCounts[h.State]++
			v.Healths[s.Key()] = h
			if s.DeadLetter != nil {
				v.WithDeadLetter++
			}
			if s.Sink.Kind != "" && !containsString(v.Kinds, s.Sink.Kind) {
				v.Kinds = append(v.Kinds, s.Sink.Kind)
			}
			if f.Kind != "" && s.Sink.Kind != f.Kind || q != "" && !strings.Contains(s.SearchText(), q) {
				continue
			}
			v.Rows = append(v.Rows, s)
		}
	}
	sort.Strings(v.Kinds)
	return v
}

// BatchIn is how a stream batches, as a row says it: "2 minutes or 2,000",
// "30 seconds", "up to 500"; "" with no buffer declared.
func (s Stream) BatchIn() i18n.Cat {
	every, records := s.Field("buffer.flush.every"), s.Field("buffer.flush.records")
	var count any = ""
	if n, err := strconv.ParseInt(records, 10, 64); err == nil {
		count = i18n.Num(n)
	}
	switch {
	case every != "" && count != "":
		return i18n.Cat{i18n.M("streams.batch.both", "every", durationWords(every), "records", count)}
	case every != "":
		return i18n.Cat{durationWords(every)}
	case count != "":
		return i18n.Cat{i18n.M("streams.batch.records", "records", count)}
	}
	return nil
}

// DeadLetterShown is the dead letter as a row draws it: the catalog's, or --
// for one whose manifest could not name it, a path relative to the gateway's
// working directory -- what the stream declares, its kind and its path, the
// reason it has no URI kept as its note.
func (s Stream) DeadLetterShown() *Place {
	if s.DeadLetter == nil {
		return nil
	}
	p := *s.DeadLetter
	if p.URI == "" {
		p.URI = s.Field("dead_letter.path")
		if p.Kind == "" {
			p.Kind = sinkKind(s.Field("dead_letter.type"))
		}
	}
	return &p
}

// sinkKind is a gateway sink's type as a destination's kind: `files` is the
// `file` scheme.
func sinkKind(t string) string {
	if t == "files" {
		return "file"
	}
	return t
}

// Note is the first line of the comment on the stream in the gateway's file
// (the one above `- name:`): what it is for, in its owner's words.
func (s Stream) Note() string {
	if s.Config == nil {
		return ""
	}
	return NoteHead(s.Config.Notes[""])
}

// streamFlow is the stream drawn as its compact flow: the configuration's,
// when it described itself (#108), else where events are posted and where
// they land. Either way the dead letter is the one the stream shows,
// hanging off the destination.
func streamFlow(s Stream) *Flow {
	f := FlowOf(s.Config, s.Sink.URI)
	if f == nil {
		f = &Flow{Nodes: []FlowNode{
			{Kind: "source", Main: plain("POST " + s.Path)},
			{Kind: "destination", Main: placeWords(s.Sink), Detail: plain(joinWords(" · ", s.Sink.Kind, s.Sink.Note))},
		}}
	}
	if dl := s.DeadLetterShown(); dl != nil {
		place := *dl
		detail := i18n.Cat{i18n.M("flow.refused")}
		if place.Note != "" {
			detail = append(detail, " · ", place.Note)
		}
		f.DeadLetter = &FlowNode{Kind: "dead_letter", Main: placeWords(place), Detail: detail}
	}
	return f
}

// placeWords is a destination's URI, or that its manifest could not name it,
// for the app to say.
func placeWords(p Place) i18n.Cat {
	if p.URI == "" {
		return i18n.Cat{i18n.M("streams.unidentified")}
	}
	return i18n.Cat{p.URI}
}

// StreamView is one stream's page.
type StreamView struct {
	Stream

	// Live is the stream's health from the source, when there is one; the
	// static Health is used otherwise.
	Live *StreamHealth

	// Configured is the stream's configuration card -- settings by section,
	// the file's notes and the history of changes -- read from the sink's
	// detail; nil when the stream described nothing.
	Configured *WriterConfigView

	// SQL says a SQL service answers for the sink, so "Query destination"
	// leads somewhere.
	SQL bool

	// ReadBy are the published endpoints that read the sink: where the data
	// goes next, as far as Brevis knows (screen-streams T3).
	ReadBy []EndpointRef
}

// BuildStream lays out a stream's page. detail is its sink's, which carries
// the configuration's history; without it the page still draws what the
// stream declares.
func BuildStream(s Stream, detail *postgres.TargetDetail) StreamView {
	v := StreamView{Stream: s}
	cfg := s.Config
	if detail != nil {
		for _, w := range detail.Writers {
			if g := w.Gateway; g != nil && g.Name == s.Gateway && g.Stream == s.Path && g.Config != nil {
				cfg = g.Config
				v.PublishedAt = g.PublishedAt
			}
		}
	}
	if cfg != nil {
		v.Config = cfg
		v.Configured = &WriterConfigView{Gateway: s.Gateway, Stream: s.Path, Role: "sink",
			PublishedAt: v.PublishedAt, Described: true, Config: BuildConfig(cfg)}
	}
	return v
}

// Queryable says the sink is something SQL can read: not a topic, not files.
//
// Not an Iceberg table either, for now: it IS a table, but the workbench
// behind this link is brevis-sql, which speaks Postgres and BigQuery. A link
// that opens a page unable to answer is worse than no link. It turns true the
// day brevis-sql reads a lake.
func (v StreamView) Queryable() bool {
	switch v.Sink.Kind {
	case "", "pubsub", "file", "iceberg":
		return false
	}
	return true
}

// QueryHref opens the workbench on the sink.
func (v StreamView) QueryHref() string { return "/sql?target=" + url.QueryEscape(v.Sink.URI) }

// Summary is one paragraph on what the stream does, from what it declares.
func (v StreamView) Summary() i18n.Cat {
	if v.Config != nil {
		return Summary(v.Config)
	}
	return i18n.Cat{i18n.M("streams.page.summary.v1", "path", v.Path, "sink", placeWords(v.Sink))}
}

// changeSaid is one publish in the history line: the first, or what it
// changed.
func changeSaid(ch ConfigChange) i18n.Msg {
	switch {
	case ch.First:
		return i18n.M("streamcfg.history.first")
	case len(ch.Lines) == 0:
		return i18n.M("config.earlier")
	}
	paths := make([]string, len(ch.Lines))
	for i, l := range ch.Lines {
		paths[i] = l.Path
	}
	return i18n.M("streamcfg.history.changed", "paths", strings.Join(paths, ", "))
}

// Lede is the page's one sentence under its name: what comes in, where it
// goes, and that refusals are kept -- three pieces, so it does not repeat the
// overview's sentence.
func (v StreamView) Lede() i18n.Cat {
	var dest any = i18n.M("streams.page.dest_unknown")
	if v.Sink.Kind != "" {
		dest = kindName(v.Sink.Kind)
	}
	lede := i18n.Cat{i18n.M("streams.page.lede", "dest", dest)}
	if f := v.Field("format"); f != "" {
		lede[0] = i18n.M("streams.page.lede.format", "format", f, "dest", dest)
	}
	if v.DeadLetter != nil {
		lede = append(lede, " ", i18n.M("streams.page.lede.dead_letter"))
	}
	return lede
}

// kindName is a destination kind's name, as its logo is read: a brand is its
// own name in every language; the two that are words are sentences the app says.
func kindName(kind string) any {
	key := kindLogo(kind)
	if key == "file" || key == "gateway" {
		return i18n.M("logo." + key)
	}
	return components.LogoTitle(key)
}

// Tables are the sink's tables for an auto_table, by the prefixes it
// accepts ("agents_*"); nil for a sink that writes one place.
func (v StreamView) Tables() []string {
	if v.Field("sink.type") != "auto_table" || v.Config == nil {
		return nil
	}
	for _, f := range v.Config.Fields {
		if f.Path == "sink.naming.allow" {
			if list, ok := f.Value.([]any); ok {
				out := make([]string, len(list))
				for i, p := range list {
					out[i] = valueText(p) + "*"
				}
				return out
			}
		}
	}
	return nil
}

// DeadLetterPanel is the commands and the resend note, when the stream's
// configuration says what its dead letter is.
func (v StreamView) DeadLetterPanel() *DeadLetterView {
	if v.Configured == nil {
		return nil
	}
	return v.Configured.Config.DeadLetter
}

// BuriedMetric is the gateway's counter of what this stream buried, labeled
// by the stream's name as the gateway labels it.
func (v StreamView) BuriedMetric() string {
	if n := v.Field("name"); n != "" {
		return `brevis_gateway_dead_letter_records_total{stream="` + n + `"}`
	}
	return "brevis_gateway_dead_letter_records_total"
}

// FindStream is the stream with this gateway and path, or nil.
func FindStream(groups []GatewayGroup, gateway, path string) *Stream {
	for _, g := range groups {
		if g.Name != gateway {
			continue
		}
		for i := range g.Streams {
			if g.Streams[i].Path == path {
				return &g.Streams[i]
			}
		}
	}
	return nil
}

// TestFormat is the stream's declared format; json, the gateway's default,
// when it declares none.
func (v StreamView) TestFormat() string {
	if f := v.Field("format"); f != "" {
		return f
	}
	return "json"
}

// TestBody is an example event in the stream's own format: an object for
// json, a list of them for array, one per line for ndjson -- with the field
// an identity routes by (`identity.source_key`) filled in, since the gateway
// refuses an event without it.
func (v StreamView) TestBody() string {
	event := `{"event": "test"}`
	if k := v.Field("identity.source_key"); k != "" {
		event = `{"event": "test", "` + k + `": "test"}`
	}
	// An auto_table event names its table, with a prefix the stream accepts.
	if v.TestTable() != "" {
		name := "test"
		if allow := v.TestAllow(); allow != "" {
			first, _, _ := strings.Cut(allow, ",")
			name = first + name
		}
		event = strings.TrimSuffix(event, "}") + `, "` + v.TestTable() + `": "` + name + `"}`
	}
	switch v.TestFormat() {
	case "array":
		return "[" + event + "]"
	case "ndjson":
		return event + "\n" + event
	}
	return event
}

// TestTable is the field an event names its table in, for an auto_table
// stream -- always `table_name`, the gateway's fixed envelope -- and "" for
// one that writes one place.
func (v StreamView) TestTable() string {
	if v.Field("sink.type") == "auto_table" {
		return "table_name"
	}
	return ""
}

// TestAllow is the prefixes an auto_table accepts, comma separated; "" is
// any name its pattern accepts.
func (v StreamView) TestAllow() string {
	if v.Config == nil {
		return ""
	}
	for _, f := range v.Config.Fields {
		if f.Path == "sink.naming.allow" {
			return strings.ReplaceAll(valueText(f.Value), ", ", ",")
		}
	}
	return ""
}

// TestDataset is where an auto_table's tables are; TestLands is where a
// stream that writes one place writes.
func (v StreamView) TestDataset() string { return v.Field("sink.into.dataset") }

func (v StreamView) TestLands() string { return v.Sink.URI }

// TestCurlHead is the curl that sends a body to this stream, up to the body:
// $GATEWAY and $GATEWAY_TOKEN stand where the console does not know the
// gateway's address or its token (`listen.auth` is the gateway's, not the
// stream's). The body follows as a heredoc, so nothing in it needs quoting.
func (v StreamView) TestCurlHead() string {
	ct := "application/json"
	if v.TestFormat() == "ndjson" {
		ct = "application/x-ndjson"
	}
	return `curl -X POST "$GATEWAY` + v.Path + `" -H 'Content-Type: ` + ct +
		`' -H "Authorization: Bearer $GATEWAY_TOKEN" --data-binary @- <<'EOF'`
}

// RowsRead is how many rows a load's step read, from its stages in that
// load's run (screen-load-v2 LV2): the landing records only what was
// written. The first map's `in` is what entered the pipeline; with no map,
// the source's own `rows`. Nil when the stages do not say -- a step without
// the SDK, or a source that counts pages, not rows.
func RowsRead(stages []postgres.Stage) *int64 {
	var source *int64
	for _, s := range stages {
		switch s.Name {
		case "map":
			if n, ok := wholeNumber(s.Numbers["in"]); ok {
				return &n
			}
		case "extract":
			if n, ok := wholeNumber(s.Numbers["rows"]); ok && source == nil {
				source = &n
			}
		}
	}
	return source
}

func wholeNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// loadsCount says how many loads the bars draw, and that it is the last
// loadsShown when the read was cut there: a count at the LIMIT is a floor.
func loadsCount(n int) i18n.Msg {
	if n >= loadsShown {
		return i18n.M("loads.drawer.recent.last", "n", fmt.Sprint(loadsShown))
	}
	return i18n.MN("loads.drawer.recent.count", n)
}

// loadWhen is when a step runs, in words where they are sure and the
// expression otherwise; the drawer writes the expression beside it anyway.
func loadWhen(w *WriterView) i18n.Cat { return cronWords(w.Cron, w.Timezone) }

// ReadNothing is a last load that wrote nothing out of something read: a
// merge that found every row already there, or a filter that kept none --
// the load that needs a look.
func (v LoadView) ReadNothing() bool {
	return v.Row.LastRows != nil && *v.Row.LastRows == 0 && v.Read != nil && *v.Read > 0
}
