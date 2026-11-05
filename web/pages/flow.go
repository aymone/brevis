package pages

import (
	"strings"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

// Flow is how data reaches a destination, as one writer's configuration
// declares it: a row of nodes from source to destination and, for a gateway
// stream, a branch where refused batches go.
//
// DRAWN ONLY FROM WHAT IS DECLARED. Every label below is a configuration
// field or the destination itself; a part the configuration does not mention
// -- a buffer, a dead letter -- is not drawn. A diagram that showed them
// anyway would be inventing the pipeline it claims to describe.
type Flow struct {
	Nodes      []FlowNode
	DeadLetter *FlowNode
}

// FlowNode is one step of a Flow. Kind is "source", "buffer", "destination"
// or "dead_letter"; Main is what it is, Detail how. Both are words for the app
// to put together (a string is the data's own, a Msg the console's): the
// buffer's triggers, the dead letter.
type FlowNode struct {
	Kind         string
	Main, Detail i18n.Cat
}

// plain is a value the data carries, as words; nil for none.
func plain(s string) i18n.Cat {
	if s == "" {
		return nil
	}
	return i18n.Cat{s}
}

// FlowOf reads a writer's configuration into a Flow, or nil when it has none
// to draw: a writer that did not describe itself (a Python, dbt or shell
// step), or a configuration that names neither a gateway's sink nor an SDK
// pipeline's source.
//
// The two shapes are the ones Summary already tells apart: a gateway stream
// (`sink.type`) and an SDK pipeline (`source.driver`).
func FlowOf(c *postgres.WriterConfig, target string) *Flow {
	if c == nil {
		return nil
	}
	f := map[string]any{}
	var order []string
	for _, fl := range c.Fields {
		f[fl.Path] = fl.Value
		order = append(order, fl.Path)
	}
	text := func(path string) string {
		if v, ok := f[path]; ok && v != nil {
			return valueText(v)
		}
		return ""
	}

	if driver := text("source.driver"); driver != "" {
		return &Flow{Nodes: []FlowNode{
			{Kind: "source", Main: plain(driver),
				Detail: plain(joinWords(" ", text("source.method"), text("source.url")))},
			{Kind: "destination", Main: plain(target),
				Detail: plain(joinWords(" · ", text("target.driver"), text("target.dedup")))},
		}}
	}

	sink := text("sink.type")
	if sink == "" {
		return nil
	}
	flow := &Flow{}
	source := FlowNode{Kind: "source", Detail: plain(text("format"))}
	switch {
	case text("path") != "":
		source.Main = plain("POST " + text("path"))
	case text("name") != "":
		source.Main = plain("stream " + text("name"))
	default:
		source.Main = plain("events")
	}
	flow.Nodes = append(flow.Nodes, source)

	// THE BUFFER, when its flush is configured: "every 2 minutes or 2,000
	// events", then any other trigger as the file names it. A switch
	// (`claim`) is not a trigger and says nothing here.
	// The ceilings a batch never passes -- its age and its size -- are said
	// as one "at most 6 minutes / 8MiB"; a trigger with no words here stays as
	// the file names it.
	every, records := text("buffer.flush.every"), text("buffer.flush.records")
	maxAge, maxSize := text("buffer.flush.max_age"), text("buffer.flush.size")
	var others []string
	for _, path := range order {
		key, ok := strings.CutPrefix(path, "buffer.flush.")
		if !ok || key == "every" || key == "records" || key == "max_age" || key == "size" {
			continue
		}
		if _, isBool := f[path].(bool); isBool {
			continue
		}
		others = append(others, key+" "+text(path))
	}
	if every != "" || records != "" || maxAge != "" || maxSize != "" || len(others) > 0 {
		var d i18n.Cat
		add := func(part ...any) {
			if len(d) > 0 {
				d = append(d, " · ")
			}
			d = append(d, part...)
		}
		switch {
		case every != "" && records != "":
			add(i18n.M("flow.buffer.both", "every", durationWords(every), "records", events(records)))
		case every != "":
			add(i18n.M("flow.buffer.every", "every", durationWords(every)))
		case records != "":
			add(i18n.M("flow.buffer.records", "records", events(records)))
		}
		if maxAge != "" || maxSize != "" {
			var what i18n.Cat
			if maxAge != "" {
				what = append(what, durationWords(maxAge))
			}
			if maxSize != "" {
				if len(what) > 0 {
					what = append(what, " / ")
				}
				what = append(what, maxSize)
			}
			add(i18n.M("flow.buffer.max", "what", what))
		}
		for _, o := range others {
			add(o)
		}
		flow.Nodes = append(flow.Nodes, FlowNode{Kind: "buffer", Main: plain(joinWords(" ", text("buffer.type"))), Detail: d})
	}

	write := text("sink.write")
	if sink == "auto_table" {
		write = text("sink.into.write")
	}
	flow.Nodes = append(flow.Nodes, FlowNode{Kind: "destination", Main: plain(target),
		Detail: plain(joinWords(" · ", sink, write))})

	if text("dead_letter.type") != "" {
		flow.DeadLetter = &FlowNode{Kind: "dead_letter", Main: i18n.Cat{destination(text, "dead_letter", true)},
			Detail: i18n.Cat{i18n.M("flow.refused")}}
	}
	return flow
}

// joinWords joins the parts that are not empty.
func joinWords(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
