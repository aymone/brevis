package pages

import (
	"brevis.sh/brevis/web/i18n"
	"strconv"
	"strings"
	"time"
)

// The Ingest screens as JSON, for the React app. Everything that is a
// decision or a sentence the server writes (a status, a schedule in words,
// a summary, a flow's detail) is finished here, in the reader's language; the
// app only lays it out. Lists are never nil.

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// PlaceJSON is a destination: where it is, split for a row, and the name of
// its kind.
type PlaceJSON struct {
	URI   string `json:"uri"`
	Kind  string `json:"kind"`
	Where string `json:"where"`
	What  string `json:"what"`
	Note  string `json:"note"`
}

func placeJSON(p Place) PlaceJSON {
	out := PlaceJSON{URI: p.URI, Kind: p.Kind, Note: p.Note}
	if p.URI != "" {
		out.Where, out.What = splitDestination(p.URI)
	}
	return out
}

// SourceJSON is where a step reads. Kind is empty when it declared none.
type SourceJSON struct {
	Kind   string `json:"kind"`
	Driver string `json:"driver"`
	// Label is the origin's own name, the same in every language (a database,
	// a driver); LabelMsg says it where the console has words for it (http,
	// files): the app says one or the other.
	Label    string    `json:"label"`
	LabelMsg *i18n.Msg `json:"labelMsg"`
	Where    string    `json:"where"`
}

func sourceJSON(s Source) SourceJSON {
	out := SourceJSON{Kind: s.Kind, Driver: s.Driver, Where: s.Where}
	if s.Kind != "" {
		out.LabelMsg, out.Label = originLabel(s)
	}
	return out
}

// LoadRowJSON is one step landing on one destination.
type LoadRowJSON struct {
	Workflow   string     `json:"workflow"`
	Step       string     `json:"step"`
	Href       string     `json:"href"`
	Target     PlaceJSON  `json:"target"`
	Legacy     bool       `json:"legacy"`
	Status     string     `json:"status"`
	LastLoaded *time.Time `json:"lastLoaded"`
	Rows       *int64     `json:"rows"`
	Schedule   i18n.Cat   `json:"schedule"`
	Source     SourceJSON `json:"source"`
	Recent     []*int64   `json:"recent"`
	Fixing     *string    `json:"fixing"`
}

// SourceChoiceJSON is one origin the loads filter offers: its key and its words.
type SourceChoiceJSON struct {
	Key   string   `json:"key"`
	Label i18n.Msg `json:"label"`
}

// ChoiceJSON is one choice of a filter: its key and its words.
type ChoiceJSON struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// LoadsFilterJSON echoes the filter the server applied.
type LoadsFilterJSON struct {
	Status string `json:"status"`
	Source string `json:"source"`
	Q      string `json:"q"`
	Legacy bool   `json:"legacy"`
}

// LoadsJSON is /api/ingest/loads. Summary counts every load the page holds,
// Counts each band choice ("" for all) under the other filters.
type LoadsJSON struct {
	Rows      []LoadRowJSON      `json:"rows"`
	Total     int                `json:"total"`
	Workflows int                `json:"workflows"`
	Summary   map[string]int     `json:"summary"`
	Counts    map[string]int     `json:"counts"`
	Sources   []SourceChoiceJSON `json:"sources"`
	HasLegacy bool               `json:"hasLegacy"`
	Filter    LoadsFilterJSON    `json:"filter"`
}

func loadRowJSON(t LoadTarget) LoadRowJSON {
	recent := t.Recent
	if recent == nil {
		recent = []*int64{}
	}
	return LoadRowJSON{
		Workflow: t.Workflow, Step: t.Step, Href: t.Href(), Target: placeJSON(t.Place),
		Legacy: t.Legacy, Status: string(t.Status), LastLoaded: ptrTime(t.LastLoaded), Rows: t.Rows,
		Schedule: loadSchedule(t), Source: sourceJSON(t.Source), Recent: recent, Fixing: t.Fixing(),
	}
}

// LoadsAsJSON lays the loads list out for the app.
func LoadsAsJSON(v LoadsView) LoadsJSON {
	out := LoadsJSON{
		Rows: make([]LoadRowJSON, 0, len(v.Rows)), Total: v.Total, Workflows: v.Workflows,
		Summary: map[string]int{}, Counts: map[string]int{}, Sources: []SourceChoiceJSON{}, HasLegacy: v.HasLegacy,
		Filter: LoadsFilterJSON{Status: v.Filter.Status, Source: v.Filter.Source, Q: v.Filter.Q, Legacy: v.Filter.Legacy},
	}
	for _, r := range v.Rows {
		out.Rows = append(out.Rows, loadRowJSON(r))
	}
	for s, n := range v.Summary {
		out.Summary[string(s)] = n
	}
	for k, n := range v.counts {
		out.Counts[k] = n
	}
	for _, s := range v.Sources {
		out.Sources = append(out.Sources, SourceChoiceJSON{Key: s, Label: sourceChoice(s)})
	}
	return out
}

// RowJSON is one configuration setting.
type RowJSON struct {
	Path    string     `json:"path"`
	Label   string     `json:"label"`
	Value   string     `json:"value"`
	Default bool       `json:"default"`
	Note    string     `json:"note"`
	List    []string   `json:"list"`
	Before  []NoteJSON `json:"before"`
}

// NoteJSON is a block's comment.
type NoteJSON struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

func rowsJSON(rows []ConfigRow) []RowJSON {
	out := make([]RowJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowJSON(r))
	}
	return out
}

func rowJSON(r ConfigRow) RowJSON {
	o := RowJSON{Path: r.Path, Label: r.Label, Value: r.Value, Default: r.Default, Note: r.Note,
		List: append([]string{}, r.List...), Before: []NoteJSON{}}
	for _, b := range r.Before {
		o.Before = append(o.Before, NoteJSON(b))
	}
	return o
}

// FlowNodeJSON and FlowJSON are a writer's flow, its words for the app to say.
type FlowNodeJSON struct {
	Kind   string   `json:"kind"`
	Main   i18n.Cat `json:"main"`
	Detail i18n.Cat `json:"detail"`
}

type FlowJSON struct {
	Nodes      []FlowNodeJSON `json:"nodes"`
	DeadLetter *FlowNodeJSON  `json:"deadLetter"`
}

func flowJSON(f *Flow) *FlowJSON {
	if f == nil {
		return nil
	}
	node := func(n FlowNode) FlowNodeJSON {
		return FlowNodeJSON(n)
	}
	out := &FlowJSON{Nodes: make([]FlowNodeJSON, 0, len(f.Nodes))}
	for _, n := range f.Nodes {
		out.Nodes = append(out.Nodes, node(n))
	}
	if f.DeadLetter != nil {
		d := node(*f.DeadLetter)
		out.DeadLetter = &d
	}
	return out
}

// ChangeJSON is one version in a configuration's history.
type ChangeJSON struct {
	At      time.Time        `json:"at"`
	RunID   string           `json:"runId"`
	First   bool             `json:"first"`
	Earlier bool             `json:"earlier"`
	Lines   []ChangeLineJSON `json:"lines"`
}

type ChangeLineJSON struct {
	Path string   `json:"path"`
	Text i18n.Cat `json:"text"`
}

func changesJSON(cs []ConfigChange) []ChangeJSON {
	out := make([]ChangeJSON, 0, len(cs))
	for _, c := range cs {
		o := ChangeJSON{At: c.At, RunID: c.RunID, First: c.First, Earlier: !c.First && len(c.Lines) == 0, Lines: []ChangeLineJSON{}}
		for _, l := range c.Lines {
			o.Lines = append(o.Lines, ChangeLineJSON{Path: l.Path, Text: l.ChangeText()})
		}
		out = append(out, o)
	}
	return out
}

// StepConfigJSON is a step's configuration as its load's page shows it, in the
// tabs' grouping.
type StepConfigJSON struct {
	Summary i18n.Cat      `json:"summary"`
	Note    string        `json:"note"`
	Flow    *FlowJSON     `json:"flow"`
	Stage   StageJSON     `json:"stage"`
	Other   []SectionJSON `json:"other"`

	SourceHead  []RowJSON      `json:"sourceHead"`
	SourceCount int            `json:"sourceCount"`
	Common      []RowJSON      `json:"common"`
	Varying     []string       `json:"varying"`
	Sources     []SourceRowsJS `json:"sources"`

	Transform []TransformJSON `json:"transform"`

	Identity    IdentityJSON `json:"identity"`
	Destination []RowJSON    `json:"destination"`

	Schema      []SchemaColJSON `json:"schema"`
	SchemaExtra []RowJSON       `json:"schemaExtra"`
	HasSchema   bool            `json:"hasSchema"`
}

// StageJSON is the path in one line: the facts, for the app to word.
type StageJSON struct {
	SourceDriver   string `json:"sourceDriver"`
	SourceCount    int    `json:"sourceCount"`
	Workers        *int   `json:"workers"`
	TransformCount int    `json:"transformCount"`
	TransformNames string `json:"transformNames"`
	TargetDriver   string `json:"targetDriver"`
	Dedup          string `json:"dedup"`
}

// SectionJSON is a block of settings: its key, and its heading where the
// console has words for it (else the app makes one of the key).
type SectionJSON struct {
	Key   string    `json:"key"`
	Title *i18n.Msg `json:"title"`
	Rows  []RowJSON `json:"rows"`
}

// SourceRowsJS is one source of a from.Many; Cells align with Varying and are
// null where the source lacks the key.
type SourceRowsJS struct {
	Index int        `json:"index"`
	Cells []*RowJSON `json:"cells"`
}

type TransformJSON struct {
	Index string    `json:"index"`
	Rows  []RowJSON `json:"rows"`
}

type IdentityJSON struct {
	Kind   string `json:"kind"`
	Legacy bool   `json:"legacy"`
	Where  string `json:"where"`
	Object string `json:"object"`
}

type SchemaColJSON struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Role i18n.Cat `json:"role"`
}

// StepConfigAsJSON is the described configuration of a step.
func StepConfigAsJSON(c WriterConfigView, target TargetView) StepConfigJSON {
	t := LoadConfigTabs(c.Config)
	out := StepConfigJSON{
		Summary: c.Config.SummaryOf(), Note: c.Config.Note, Flow: flowJSON(c.Flow),
		Other:      []SectionJSON{},
		SourceHead: rowsJSON(t.SourceHead), SourceCount: t.SourceCount, Common: rowsJSON(t.Common),
		Varying: append([]string{}, t.Varying...), Sources: []SourceRowsJS{},
		Transform:   []TransformJSON{},
		Destination: rowsJSON(t.Destination), Schema: []SchemaColJSON{}, SchemaExtra: rowsJSON(schemaExtra(t.SchemaRows)),
		HasSchema: len(t.SchemaRows) > 0,
	}
	for _, s := range t.Other {
		out.Other = append(out.Other, SectionJSON{Key: s.Key, Title: s.TitleMsg(), Rows: rowsJSON(s.Rows)})
	}
	for _, s := range t.Sources {
		line := SourceRowsJS{Index: s.Index, Cells: make([]*RowJSON, len(s.Cells))}
		for i, cell := range s.Cells {
			if cell != nil {
				r := rowJSON(*cell)
				line.Cells[i] = &r
			}
		}
		out.Sources = append(out.Sources, line)
	}
	for _, op := range t.Transform {
		out.Transform = append(out.Transform, TransformJSON{Index: op.Index, Rows: rowsJSON(op.Rows)})
	}
	for _, col := range t.Schema {
		out.Schema = append(out.Schema, SchemaColJSON{Name: col.Name, Type: col.Type, Role: schemaRole(col.Role)})
	}
	out.Stage = StageJSON{
		SourceDriver: rowValue(t.SourceHead, "driver"), SourceCount: t.SourceCount,
		TransformCount: len(t.Transform), TransformNames: transformNames(t),
		TargetDriver: rowValue(t.Destination, "driver"), Dedup: rowValue(t.Destination, "dedup"),
	}
	if w, err := strconv.Atoi(rowValue(t.SourceHead, "workers")); err == nil && t.SourceCount > 0 {
		out.Stage.Workers = &w
	}
	where, what := SplitTarget(target.Row.Target)
	out.Identity = IdentityJSON{Kind: target.Row.Kind, Legacy: target.Row.Legacy, Where: where, Object: what}
	return out
}

// BarJSON is one load bar; LoadRecordJSON one row of the recent loads.
type BarJSON struct {
	RunID string `json:"runId"`
	Rows  *int64 `json:"rows"`
	Title string `json:"title"`
}

type LoadRecordJSON struct {
	LoadedAt time.Time `json:"loadedAt"`
	Rows     *int64    `json:"rows"`
	Bytes    *int64    `json:"bytes"`
	RunID    string    `json:"runId"`
}

// ReaderJSON is a published endpoint that reads a destination.
type ReaderJSON struct {
	API  string `json:"api"`
	Name string `json:"name"`
	Path string `json:"path"`
	Href string `json:"href"`
}

func readersJSON(rs []EndpointRef) []ReaderJSON {
	out := make([]ReaderJSON, 0, len(rs))
	for _, r := range rs {
		out = append(out, ReaderJSON(r))
	}
	return out
}

// LoadChoiceJSON is a destination a step writes, the open one marked.
type LoadChoiceJSON struct {
	URI     string `json:"uri"`
	Current bool   `json:"current"`
}

// LoadDetailJSON is the open load.
type LoadDetailJSON struct {
	Status      string           `json:"status"`
	Reason      i18n.Cat         `json:"reason"`
	Source      SourceJSON       `json:"source"`
	Target      PlaceJSON        `json:"target"`
	HasSchedule bool             `json:"hasSchedule"`
	When        i18n.Cat         `json:"when"`
	Cron        string           `json:"cron"`
	Timezone    string           `json:"timezone"`
	Legacy      bool             `json:"legacy"`
	Manual      bool             `json:"manual"`
	LastLoaded  *time.Time       `json:"lastLoaded"`
	LastRows    *int64           `json:"lastRows"`
	Read        *int64           `json:"read"`
	ReadNothing bool             `json:"readNothing"`
	Fixing      *string          `json:"fixing"`
	Bars        []BarJSON        `json:"bars"`
	Missed      int              `json:"missed"`
	Loads       []LoadRecordJSON `json:"loads"`
	LoadsCount  i18n.Msg         `json:"loadsCount"`
	Described   bool             `json:"described"`
	Config      *StepConfigJSON  `json:"config"`
	Changes     []ChangeJSON     `json:"changes"`
	ReadBy      []ReaderJSON     `json:"readBy"`
	Query       string           `json:"query"`
	Run         string           `json:"run"`
}

// LoadJSON is /api/ingest/loads/{workflow}/{step}. Choosing is a step that
// writes several destinations with none open: Detail is then null.
type LoadJSON struct {
	Workflow string           `json:"workflow"`
	Step     string           `json:"step"`
	Choices  []LoadChoiceJSON `json:"choices"`
	Choosing bool             `json:"choosing"`
	Detail   *LoadDetailJSON  `json:"detail"`
}

// LoadAsJSON lays one load out for the app.
func LoadAsJSON(l LoadView) LoadJSON {
	out := LoadJSON{Workflow: l.Workflow, Step: l.Step, Choices: []LoadChoiceJSON{}, Choosing: l.Choosing()}
	for _, c := range l.Choices {
		out.Choices = append(out.Choices, LoadChoiceJSON{URI: c.URI, Current: c.Current})
	}
	if out.Choosing {
		return out
	}
	d := &LoadDetailJSON{
		Status: string(l.Row.Status), Source: sourceJSON(l.Source), Target: placeJSON(Place{URI: l.Row.Target, Kind: l.Row.Kind}),
		LastLoaded: l.Row.LastWhen, LastRows: l.Row.LastRows, Read: l.Read, ReadNothing: l.ReadNothing(),
		Bars: make([]BarJSON, 0, len(l.Bars)), Missed: l.Missed, Loads: make([]LoadRecordJSON, 0, len(l.Loads)),
		LoadsCount: loadsCount(len(l.Loads)), Changes: []ChangeJSON{}, ReadBy: readersJSON(l.ReadBy),
		Run: l.Run, Legacy: l.Row.Legacy,
	}
	if w := l.Writer(); w != nil {
		d.Reason, d.Fixing = w.Reason(), w.RunInFlight
		d.HasSchedule, d.Cron, d.Timezone = w.HasSchedule, w.Cron, w.Timezone
		if w.HasSchedule {
			d.When = loadWhen(w)
		}
	}
	d.Manual = !d.HasSchedule
	for _, b := range l.Bars {
		d.Bars = append(d.Bars, BarJSON{RunID: strings.TrimPrefix(b.Href, "/runs/"), Rows: b.Rows, Title: b.Title})
	}
	for _, x := range l.Loads {
		d.Loads = append(d.Loads, LoadRecordJSON{LoadedAt: x.LoadedAt, Rows: x.Rows, Bytes: x.Bytes, RunID: x.RunID})
	}
	if l.Tabs {
		d.Query = l.WorkbenchHref()
	}
	if c, ok := l.StepConfig(); ok {
		d.Described = c.Described
		if c.Described {
			cfg := StepConfigAsJSON(c, l.TargetView)
			d.Config = &cfg
			d.Changes = changesJSON(c.Config.Changes)
		}
	}
	out.Detail = d
	return out
}

// StreamRowJSON is one gateway stream.
type StreamRowJSON struct {
	Gateway string    `json:"gateway"`
	Path    string    `json:"path"`
	Href    string    `json:"href"`
	Note    string    `json:"note"`
	Sink    PlaceJSON `json:"sink"`
	Batch   i18n.Cat  `json:"batch"`
	// BatchEvery and BatchRecords are the same batch as the declared values ("2m", 2000), for a
	// compact cell that reads the same in every language; Batch is the sentence for a tooltip.
	BatchEvery   string     `json:"batchEvery"`
	BatchRecords *int64     `json:"batchRecords"`
	DeadLetter   *PlaceJSON `json:"deadLetter"`
	// Health is added beside the old fields: a client that does not know it ignores it.
	Health StreamHealth `json:"health"`
}

// StreamsJSON is /api/ingest/streams.
type StreamsJSON struct {
	Rows           []StreamRowJSON `json:"rows"`
	Total          int             `json:"total"`
	Gateways       int             `json:"gateways"`
	WithDeadLetter int             `json:"withDeadLetter"`
	// HealthCounts is every stream's health state, whatever the filter keeps.
	HealthCounts map[string]int `json:"healthCounts"`
	Kinds        []string       `json:"kinds"`
	Filter       struct {
		Kind string `json:"kind"`
		Q    string `json:"q"`
	} `json:"filter"`
}

func streamRowJSON(s Stream, h StreamHealth) StreamRowJSON {
	out := StreamRowJSON{Gateway: s.Gateway, Path: s.Path, Href: s.Href(), Note: s.Note(),
		Sink: placeJSON(s.Sink), Batch: s.BatchIn(), BatchEvery: s.Field("buffer.flush.every"), Health: h}
	if n, err := strconv.ParseInt(s.Field("buffer.flush.records"), 10, 64); err == nil {
		out.BatchRecords = &n
	}
	if dl := s.DeadLetterShown(); dl != nil {
		p := placeJSON(*dl)
		out.DeadLetter = &p
	}
	return out
}

// StreamsAsJSON lays the streams list out for the app.
func StreamsAsJSON(v StreamsView) StreamsJSON {
	out := StreamsJSON{Rows: make([]StreamRowJSON, 0, len(v.Rows)), Total: v.Total, Gateways: v.Gateways,
		WithDeadLetter: v.WithDeadLetter, HealthCounts: v.HealthCounts, Kinds: append([]string{}, v.Kinds...)}
	out.Filter.Kind, out.Filter.Q = v.Filter.Kind, v.Filter.Q
	for _, s := range v.Rows {
		h, ok := v.Healths[s.Key()]
		if !ok {
			h = s.Health()
		}
		out.Rows = append(out.Rows, streamRowJSON(s, h))
	}
	return out
}

// ExplainedRowJSON and CardJSON are the stream's settings in cards.
type ExplainedRowJSON struct {
	Path    string    `json:"path"`
	Label   *i18n.Msg `json:"label"`
	Value   string    `json:"value"`
	Default bool      `json:"default"`
	Note    string    `json:"note"`
}

type CardJSON struct {
	Key   string             `json:"key"`
	Title i18n.Msg           `json:"title"`
	Rows  []ExplainedRowJSON `json:"rows"`
}

// DeadLetterJSON is the dead letter panel: where it is and the commands.
type DeadLetterJSON struct {
	Where    i18n.Cat `json:"where"`
	Commands string   `json:"commands"`
	Readable bool     `json:"readable"`
	Write    string   `json:"write"`
}

// TestEventJSON is the example event and the curl that sends it.
type TestEventJSON struct {
	Format   string `json:"format"`
	Body     string `json:"body"`
	CurlHead string `json:"curlHead"`
	Lands    string `json:"lands"`
	Table    string `json:"table"`
	// Allow and Dataset are what an auto_table's test event is checked by: the
	// prefixes a table name may start with ([] is any) and where the tables are.
	Allow   []string `json:"allow"`
	Dataset string   `json:"dataset"`
}

// StreamChangeJSON is one publish in a stream's history line: the first, or what it changed.
type StreamChangeJSON struct {
	At   time.Time `json:"at"`
	Text i18n.Msg  `json:"text"`
}

// StreamJSON is /api/ingest/streams/{gateway}/{path...}.
type StreamJSON struct {
	Gateway      string             `json:"gateway"`
	Path         string             `json:"path"`
	Sink         PlaceJSON          `json:"sink"`
	DeadLetter   *PlaceJSON         `json:"deadLetter"`
	PublishedAt  *time.Time         `json:"publishedAt"`
	Format       string             `json:"format"`
	Health       StreamHealth       `json:"health"`
	Lede         i18n.Cat           `json:"lede"`
	Summary      i18n.Cat           `json:"summary"`
	Flow         *FlowJSON          `json:"flow"`
	Tables       []string           `json:"tables"`
	CatalogHref  string             `json:"catalogHref"`
	Query        string             `json:"query"`
	ReadBy       []ReaderJSON       `json:"readBy"`
	HasConfig    bool               `json:"hasConfig"`
	Cards        []CardJSON         `json:"cards"`
	YAML         string             `json:"yaml"`
	Changes      []StreamChangeJSON `json:"changes"`
	DeadPanel    *DeadLetterJSON    `json:"deadLetterPanel"`
	BuriedMetric string             `json:"buriedMetric"`
	TestEvent    TestEventJSON      `json:"testEvent"`
}

// StreamAsJSON lays one stream's page out for the app.
func StreamAsJSON(v StreamView) StreamJSON {
	out := StreamJSON{
		Gateway: v.Gateway, Path: v.Path, Sink: placeJSON(v.Sink), PublishedAt: ptrTime(v.PublishedAt),
		Format: v.Field("format"), Health: v.healthNow(), Lede: v.Lede(), Summary: v.Summary(), Flow: flowJSON(streamFlow(v.Stream)),
		Tables: append([]string{}, v.Tables()...), ReadBy: readersJSON(v.ReadBy), HasConfig: v.Config != nil,
		Cards: []CardJSON{}, Changes: []StreamChangeJSON{}, BuriedMetric: v.BuriedMetric(),
		TestEvent: TestEventJSON{Format: v.TestFormat(), Body: v.TestBody(), CurlHead: v.TestCurlHead(),
			Lands: v.TestLands(), Table: v.TestTable(), Allow: splitList(v.TestAllow()), Dataset: v.TestDataset()},
	}
	if dl := v.DeadLetterShown(); dl != nil {
		p := placeJSON(*dl)
		out.DeadLetter = &p
	}
	if v.Sink.URI != "" {
		out.CatalogHref = TargetHref(v.Sink.URI)
	}
	if v.SQL && v.Queryable() {
		out.Query = v.QueryHref()
	}
	if v.Config != nil {
		out.YAML = ConfigYAML(v.Config)
		for _, c := range ExplainConfig(v.Config) {
			card := CardJSON{Key: c.Key, Title: cardTitle(c.Key), Rows: make([]ExplainedRowJSON, 0, len(c.Rows))}
			for _, r := range c.Rows {
				card.Rows = append(card.Rows, ExplainedRowJSON{Path: r.Path, Label: r.LabelMsg(), Value: r.Value, Default: r.Default, Note: r.Note})
			}
			out.Cards = append(out.Cards, card)
		}
	}
	if v.Configured != nil {
		for _, ch := range v.Configured.Config.Changes {
			out.Changes = append(out.Changes, StreamChangeJSON{At: ch.At, Text: changeSaid(ch)})
		}
	}
	if p := v.DeadLetterPanel(); p != nil {
		out.DeadPanel = &DeadLetterJSON{Where: p.Where, Commands: p.Commands(), Readable: p.Readable, Write: p.Write}
	}
	return out
}

// splitList is a comma separated list as a slice; nothing is an empty one, so
// the JSON says [] and not null.
func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (v StreamView) healthNow() StreamHealth {
	if v.Live != nil {
		return *v.Live
	}
	return v.Health()
}
