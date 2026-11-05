package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

type gatewayRef struct {
	Name   string `json:"name"`
	Stream string `json:"stream"`
}

type targetWriterView struct {
	Name string `json:"name"`
	// Gateway is set for a gateway's stream, null for a workflow step.
	Gateway     *gatewayRef `json:"gateway"`
	Workflow    string      `json:"workflow"`
	Node        string      `json:"node"`
	Status      string      `json:"status"`
	Cron        *i18n.Msg   `json:"cron"` // the schedule in words, for the app to say
	RunInFlight *string     `json:"runInFlight"`
	// Reason is why the writer stands where it does, in the writer's own
	// timezone: sentences for the app to say, read as one passage.
	Reason i18n.Cat `json:"reason"`
	Href   string   `json:"href"`
}

type loadBarView struct {
	Href  string `json:"href"`
	Rows  *int64 `json:"rows"`
	Title string `json:"title"`
}

type readerView struct {
	API  string `json:"api"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type targetDetailView struct {
	Row     catalogRowView     `json:"row"`
	Writers []targetWriterView `json:"writers"`
	Bars    []loadBarView      `json:"bars"`
	Missed  int                `json:"missed"`
	ReadBy  []readerView       `json:"readBy"`
	Configs []writerConfigView `json:"configs"`
	// SQL says the workbench can be asked about this destination.
	SQL bool `json:"sql"`
}

// apiCatalogTarget is the drawer's data: one destination, its writers and why,
// its last loads, what reads it and how each writer is configured. A
// destination nobody writes is a 404, which the app words as the catalog's
// "no destination" note.
func (u *UI) apiCatalogTarget(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("u")
	if target == "" || len(target) > targetCeiling || strings.ContainsFunc(target, unicode.IsControl) {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest)
		return
	}
	if u.catalog == nil {
		dataFail(w, http.StatusNotFound, codeNotEnabled)
		return
	}
	d, err := u.catalog.CatalogTarget(r.Context(), target)
	if err != nil {
		u.log.Error("reading a destination", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	if d == nil {
		dataFail(w, http.StatusNotFound, codeNotWritten)
		return
	}
	t := pages.BuildTarget(*d, time.Now())
	if err := u.targetTools(r, &t, target); err != nil {
		u.log.Error("reading what reads a destination", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	v := targetDetailView{
		Row: catalogRowOf(t.Row), Writers: make([]targetWriterView, 0, len(t.Row.Writers)),
		Bars: make([]loadBarView, 0, len(t.Bars)), Missed: t.Missed, ReadBy: make([]readerView, 0, len(t.ReadBy)),
		Configs: targetConfigs(t.Configs), SQL: t.Tabs,
	}
	for _, wr := range t.Row.Writers {
		tw := targetWriterView{
			Name: catWriterName(wr.CatalogWriter), Workflow: wr.Workflow, Node: wr.Node,
			Status: string(wr.Verdict.Status), RunInFlight: wr.RunInFlight, Reason: wr.Reason(),
		}
		if g := wr.Gateway; g != nil {
			tw.Gateway = &gatewayRef{Name: g.Name, Stream: g.Stream}
			tw.Href = pages.Stream{Gateway: g.Name, Path: g.Stream}.Href()
		} else {
			tw.Cron = pages.CronMsg(wr.Cron)
			tw.Href = "/ingest/loads/" + url.PathEscape(wr.Workflow) + "/" + url.PathEscape(wr.Node)
		}
		v.Writers = append(v.Writers, tw)
	}
	for _, b := range t.Bars {
		v.Bars = append(v.Bars, loadBarView{Href: b.Href, Rows: b.Rows, Title: b.Title})
	}
	for _, e := range t.ReadBy {
		v.ReadBy = append(v.ReadBy, readerView{API: e.API, Name: e.Name, Path: e.Path})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

// catWriterName is a writer as a line: `workflow · step`, or `gateway name · path`.
func catWriterName(w postgres.CatalogWriter) string {
	if g := w.Gateway; g != nil {
		return "gateway " + g.Name + " · " + g.Stream
	}
	return w.Workflow + " · " + w.Node
}

// noteView is a comment from the config file: a one-line note is shown whole,
// a longer one folds to its first line.
type noteView struct {
	Head string `json:"head"`
	Rest string `json:"rest"`
}

func catNote(s string) *noteView {
	if s == "" {
		return nil
	}
	return &noteView{Head: pages.NoteHead(s), Rest: pages.NoteRest(s)}
}

// flowNodeView is one box of a writer's flow. Kind is source, buffer,
// destination or dead_letter: the app words the heading (flow.<kind>); Main
// and Detail are the writer's own words and the console's, for the app to say.
type flowNodeView struct {
	Kind   string   `json:"kind"`
	Main   i18n.Cat `json:"main"`
	Detail i18n.Cat `json:"detail"`
}

type deadLetterView struct {
	Where    i18n.Cat `json:"where"`
	Readable bool     `json:"readable"`
	Commands string   `json:"commands"`
	// Write is append, merge or empty: which sentence on resending applies.
	Write string `json:"write"`
}

type configBefore struct {
	Label string   `json:"label"`
	Note  noteView `json:"note"`
}

type configRowView struct {
	Path    string         `json:"path"`
	Label   string         `json:"label"`
	Value   string         `json:"value"`
	Default bool           `json:"default"`
	Note    *noteView      `json:"note"`
	Before  []configBefore `json:"before"`
}

type configSectionView struct {
	Key   string          `json:"key"`
	Title *i18n.Msg       `json:"title"` // null for a block nobody named: the app makes a heading of its key
	Note  *noteView       `json:"note"`
	Rows  []configRowView `json:"rows"`
}

type changeLineView struct {
	Path string   `json:"path"`
	Text i18n.Cat `json:"text"`
}

type configChangeView struct {
	At    time.Time        `json:"at"`
	RunID string           `json:"runId"`
	First bool             `json:"first"`
	Lines []changeLineView `json:"lines"`
}

// writerConfigView is one writer's configuration: the summary, the flow, the
// dead letter and the changes are sentences the app says, built from the
// writer's published fields.
type writerConfigView struct {
	Key         string              `json:"key"`
	Gateway     string              `json:"gateway"`
	Stream      string              `json:"stream"`
	Role        string              `json:"role"`
	PublishedAt *time.Time          `json:"publishedAt"`
	Workflow    string              `json:"workflow"`
	Node        string              `json:"node"`
	Described   bool                `json:"described"`
	Summary     i18n.Cat            `json:"summary"`
	Flow        []flowNodeView      `json:"flow"`
	FlowDead    *flowNodeView       `json:"flowDeadLetter"`
	DeadLetter  *deadLetterView     `json:"deadLetter"`
	Note        *noteView           `json:"note"`
	Sections    []configSectionView `json:"sections"`
	Changes     []configChangeView  `json:"changes"`
}

func targetConfigs(in []pages.WriterConfigView) []writerConfigView {
	out := make([]writerConfigView, 0, len(in))
	for _, c := range in {
		v := writerConfigView{
			Key: c.Key(), Gateway: c.Gateway, Stream: c.Stream, Role: c.Role, Workflow: c.Workflow, Node: c.Node,
			Described: c.Described, Flow: []flowNodeView{}, Sections: []configSectionView{}, Changes: []configChangeView{},
		}
		if c.Gateway != "" {
			at := c.PublishedAt
			v.PublishedAt = &at
		}
		if !c.Described {
			out = append(out, v)
			continue
		}
		v.Summary = c.Config.SummaryOf()
		if c.Flow != nil {
			for _, n := range c.Flow.Nodes {
				v.Flow = append(v.Flow, flowNodeView(n))
			}
			if dl := c.Flow.DeadLetter; dl != nil {
				d := flowNodeView(*dl)
				v.FlowDead = &d
			}
		}
		// The dead-letter panel is a gateway stream's only.
		if dl := c.Config.DeadLetter; c.Gateway != "" && dl != nil {
			v.DeadLetter = &deadLetterView{Where: dl.Where, Readable: dl.Readable, Commands: dl.Commands(), Write: dl.Write}
		}
		v.Note = catNote(c.Config.Note)
		for _, s := range c.Config.Sections {
			sv := configSectionView{Key: s.Key, Title: s.TitleMsg(), Note: catNote(s.Note), Rows: make([]configRowView, 0, len(s.Rows))}
			for _, r := range s.Rows {
				rv := configRowView{Path: r.Path, Label: r.Label, Value: r.Value, Default: r.Default, Note: catNote(r.Note), Before: []configBefore{}}
				for _, b := range r.Before {
					rv.Before = append(rv.Before, configBefore{Label: b.Label, Note: noteView{Head: pages.NoteHead(b.Text), Rest: pages.NoteRest(b.Text)}})
				}
				sv.Rows = append(sv.Rows, rv)
			}
			v.Sections = append(v.Sections, sv)
		}
		for _, ch := range c.Config.Changes {
			cv := configChangeView{At: ch.At, RunID: ch.RunID, First: ch.First, Lines: []changeLineView{}}
			for _, l := range ch.Lines {
				cv.Lines = append(cv.Lines, changeLineView{Path: l.Path, Text: l.ChangeText()})
			}
			v.Changes = append(v.Changes, cv)
		}
		out = append(out, v)
	}
	return out
}
