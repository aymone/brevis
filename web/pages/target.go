package pages

import (
	"fmt"
	"net/url"
	"slices"
	"time"

	"brevis.sh/brevis/web/i18n"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
)

// missedShown bounds the dashed bars after the last load: enough to see a
// pipeline stopped, not ten thousand ticks of a minutely one stopped for a week.
const missedShown = 6

// TargetView is one destination's page.
type TargetView struct {
	Row DataRow

	// Bars are the last loads, oldest first, each linking to its run; Missed
	// are the slots that went by since, past their grace, drawn dashed.
	Bars   []components.LoadBar
	Missed int

	// Loads are the same loads, newest first, for the table.
	Loads []postgres.TargetLoad

	// Configs are how this destination's writers are configured, one per
	// writer that described itself.
	Configs []WriterConfigView

	// Tabs says there is a SQL service to ask about this destination at
	// all, for the link to the workbench. Without one there is no link: one
	// that always answers "not configured" is a question nobody can act on.
	Tabs bool

	// ReadBy are the published endpoints whose SQL reads this target: its
	// consumers, beside its writers.
	ReadBy []EndpointRef
}

// WorkbenchHref opens `/sql` on this destination.
//
// A TARGET IN A URL IS FINE AND A STATEMENT IS NOT, which is the distinction
// the workbench's POST keeps: this console already puts a target in
// `/data/target?u=`, and a query in a link would be a query in a proxy log.
func (v TargetView) WorkbenchHref() string {
	return "/sql?target=" + url.QueryEscape(v.Row.Target)
}

// Cell is one value, as a grid shows it.
//
// NULL IS NOT THE EMPTY STRING, and this is the last place that can still
// tell them apart: one is "nothing was recorded" and the other is a value
// somebody wrote. An em dash for the first, nothing for the second.
func Cell(v any) string {
	if v == nil {
		// THE WORD, AND NOT A DASH. Both distinguish a NULL from an empty
		// string, which is what this has always been for; a dash makes the
		// reader guess which of the two it is, and in a column of text it
		// could be a value somebody wrote. `null` is what the warehouse
		// calls it and what every console reading one prints.
		return "null"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// BuildTarget judges the destination as the list does, and lays out its loads.
func BuildTarget(d postgres.TargetDetail, now time.Time) TargetView {
	v := TargetView{Row: BuildData([]postgres.CatalogEntry{d.CatalogEntry}, now).Rows[0], Loads: d.Loads}
	for _, w := range d.Writers {
		switch g := w.Gateway; {
		case g != nil && g.Config != nil:
			v.Configs = append(v.Configs, WriterConfigView{Gateway: g.Name, Stream: g.Stream,
				Role: g.Role, PublishedAt: g.PublishedAt, Described: true, Config: BuildConfig(g.Config),
				Flow: FlowOf(g.Config, d.Target)})
		case g == nil:
			c := WriterConfigView{Workflow: w.Workflow, Node: w.Node, Described: w.Config != nil}
			if w.Config != nil {
				c.Config = BuildConfig(w.Config)
				c.Flow = FlowOf(w.Config, d.Target)
			}
			v.Configs = append(v.Configs, c)
		}
	}
	for _, l := range slices.Backward(d.Loads) {
		v.Bars = append(v.Bars, components.LoadBar{
			Href:  "/runs/" + l.RunID,
			Rows:  l.Rows,
			Title: fmt.Sprintf("%s · %s · %s", l.LoadedAt.UTC().Format("2006-01-02 15:04 UTC"), l.Workflow, rowsText(l.Rows)),
		})
	}
	// The slots the destination missed are its healthiest writer's: if any
	// writer is on time, nothing is missing.
	for _, w := range v.Row.Writers {
		if w.Verdict.Status == v.Row.Status {
			v.Missed = len(catalog.MissedSlots(writerOf(w.CatalogWriter), now, missedShown))
			break
		}
	}
	return v
}

// Reason is why a writer stands where it does, in its own schedule's
// timezone: sentences for the app to say, one after the other; none for a
// writer that stands where it should.
func (w WriterView) Reason() i18n.Cat {
	if g := w.Gateway; g != nil {
		why := i18n.Cat{i18n.M("catalog.why.gateway", "gateway", g.Name, "stream", g.Stream, "role", g.Role,
			"at", stampIn(g.PublishedAt.UTC()))}
		if g.Note != "" {
			why = append(why, " ", i18n.M("catalog.why.unnamed", "note", g.Note))
		}
		return why
	}
	loc, err := time.LoadLocation(w.Timezone)
	if err != nil || w.Timezone == "" {
		loc = time.UTC
	}

	switch w.Verdict.Status {
	case catalog.Late, catalog.Stale:
		grace := w.Verdict.Grace
		return i18n.Cat{i18n.M("catalog.why.missed", "workflow", w.Workflow, "slot", stampIn(w.Verdict.Missed.In(loc)),
			"by", stampIn(w.Verdict.Missed.Add(grace).In(loc)), "grace", i18n.Dur(grace))}
	case catalog.Paused:
		return i18n.Cat{i18n.M("catalog.why.paused")}
	case catalog.Unscheduled:
		if w.HasSchedule {
			return i18n.Cat{i18n.M("catalog.why.unreadable")}
		}
		return i18n.Cat{i18n.M("catalog.why.manual")}
	}
	return nil
}

// stampIn is an instant as a verdict says it: the date and time in its own
// zone, and the zone's name beside it.
func stampIn(t time.Time) i18n.Msg {
	return i18n.M("catalog.why.at", "at", i18n.At{T: t, Layout: "brief"}, "zone", t.Format("MST"))
}
