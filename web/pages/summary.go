package pages

import (
	"strconv"
	"strings"
	"time"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

// Summary is a writer's configuration in one sentence -- a gateway stream's or
// an SDK pipeline's, for somebody who does
// not read YAML: what comes in, where it goes, how it is written, how often,
// and where what is refused goes.
//
// WRITTEN FROM THE SETTINGS, NEVER GENERATED. Every word here is a template
// filled from a field the gateway published, so the sentence is wrong only
// when the configuration is -- and a setting it has no words for is left out
// rather than guessed at. The full list is right below it.
func Summary(c *postgres.WriterConfig) i18n.Cat {
	f := map[string]any{}
	for _, fl := range c.Fields {
		f[fl.Path] = fl.Value
	}
	text := func(path string) string {
		if v, ok := f[path]; ok && v != nil {
			return valueText(v)
		}
		return ""
	}

	if text("source.driver") != "" {
		return pipelineSummary(c.Fields, text)
	}

	sink := text("sink.type")
	if sink == "" {
		return i18n.Cat{i18n.M("summary.stream.nowhere")}
	}

	b := i18n.Cat{intake(text("format"))}
	if p := text("path"); p != "" {
		b = append(b, sp(), i18n.M("summary.stream.posted", "path", p))
	}

	switch sink {
	case "auto_table":
		var tables any = i18n.M("summary.stream.tables.each")
		if allow, ok := f["sink.naming.allow"].([]any); ok && len(allow) > 0 {
			names := make([]string, len(allow))
			for i, a := range allow {
				names[i] = valueText(a) + "*"
			}
			tables = i18n.M("summary.stream.tables.named", "names", orList(names))
		}
		b = append(b, sp(), i18n.M("summary.stream.become", "tables", tables))
		if into := destination(text, "sink.into", false); into != nil {
			b = append(b, sp(), i18n.M("summary.stream.in", "into", into))
		}
	case "pubsub":
		b = append(b, sp(), i18n.M("summary.stream.published", "to", destination(text, "sink", true)))
	case "files":
		b = append(b, sp(), i18n.M("summary.stream.written", "to", destination(text, "sink", true)))
	case "bigquery", "postgres", "mysql", "redshift", "iceberg":
		b = append(b, sp(), i18n.M("summary.stream.land", "to", destination(text, "sink", true)))
	default:
		b = append(b, sp(), i18n.M("summary.stream.sink", "sink", sink))
	}

	write := text("sink.write")
	if write == "" {
		write = text("sink.into.write")
	}
	switch write {
	case "append":
		b = append(b, comma(), i18n.M("summary.stream.append"))
	case "merge":
		b = append(b, comma(), i18n.M("summary.stream.merge"))
	}

	every, records := text("buffer.flush.every"), text("buffer.flush.records")
	switch {
	case every != "" && records != "":
		b = append(b, comma(), i18n.M("summary.stream.batch.both", "every", durationWords(every), "records", events(records)))
	case every != "":
		b = append(b, comma(), i18n.M("summary.stream.batch.every", "every", durationWords(every)))
	case records != "":
		b = append(b, comma(), i18n.M("summary.stream.batch.records", "records", events(records)))
	}

	if dl := destination(text, "dead_letter", true); dl != nil {
		b = append(b, semi(), i18n.M("summary.stream.refused", "to", dl))
	}
	return append(b, stop())
}

// intake is what a stream accepts, by its declared format.
func intake(format string) i18n.Msg {
	switch format {
	case "json":
		return i18n.M("summary.intake.json")
	case "array":
		return i18n.M("summary.intake.array")
	case "ndjson":
		return i18n.M("summary.intake.ndjson")
	}
	return i18n.M("summary.intake.events")
}

// destination names a sink in words: "BigQuery table p.d.t", "Pub/Sub topic
// p/t", "files at ./dead-letter/". withTable says whether a table is part of
// it -- auto_table's `into` names where tables go, not one of them. It is a
// Msg, or the engine's own name (a string), or nil for none.
func destination(text func(string) string, at string, withTable bool) any {
	get := func(k string) string { return text(at + "." + k) }
	table := ""
	if withTable {
		table = get("table")
	}
	switch t := get("type"); t {
	case "":
		return nil
	case "bigquery":
		where := joinWords(".", get("project"), get("dataset"), table)
		if table != "" {
			return i18n.M("summary.dest.bigquery.table", "where", where)
		}
		return i18n.M("summary.dest.bigquery", "where", where)
	case "postgres", "mysql", "redshift":
		name := map[string]string{"postgres": "Postgres", "mysql": "MySQL", "redshift": "Redshift"}[t]
		if table != "" {
			return i18n.M("summary.dest.table", "engine", name, "table", table)
		}
		return name
	case "iceberg":
		// The catalog's name qualifies the table, the way a project qualifies
		// a BigQuery dataset: the same bronze.events can live in two catalogs.
		if table != "" {
			return i18n.M("summary.dest.table", "engine", "Iceberg", "table", joinWords(".", get("catalog"), table))
		}
		return "Iceberg"
	case "pubsub":
		return i18n.M("summary.dest.topic", "topic", joinWords("/", get("project"), get("topic")))
	case "files":
		return i18n.M("summary.dest.files", "path", get("path"))
	default:
		return i18n.M("summary.dest.other", "type", t)
	}
}

// orList is "a", "a or b", "a, b or c": the pieces, for the app to say.
func orList(items []string) i18n.Cat {
	var c i18n.Cat
	for i, it := range items {
		switch {
		case i == 0:
		case i == len(items)-1:
			c = append(c, sp(), i18n.M("summary.or"), sp())
		default:
			c = append(c, comma())
		}
		c = append(c, it)
	}
	return c
}

// durationWords is a duration a person says: "1 minute", "500 milliseconds" (a
// Msg). Anything that is not one whole unit stays as written (a string).
// DurationCat is durationWords for the API: the duration said, or the raw string, as a Cat (null for none).
func DurationCat(s string) i18n.Cat {
	if s == "" {
		return nil
	}
	return i18n.Cat{durationWords(s)}
}

func durationWords(s string) any {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return s
	}
	switch {
	case d%time.Hour == 0:
		return i18n.MN("summary.unit.hour", int(d/time.Hour))
	case d%time.Minute == 0:
		return i18n.MN("summary.unit.minute", int(d/time.Minute))
	case d%time.Second == 0:
		return i18n.MN("summary.unit.second", int(d/time.Second))
	case d%time.Millisecond == 0:
		return i18n.MN("summary.unit.millisecond", int(d/time.Millisecond))
	}
	return s
}

// events is a count of events, "1 event" or "2,000 events" (a Msg; the app
// separates the thousands the reader's way), or the text as written.
func events(n string) any {
	v, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		return n
	}
	return i18n.MN("summary.events", int(v), "n", i18n.Num(v))
}

// pipelineSummary is an SDK pipeline in one sentence: what it reads, what it
// does to the records, where it writes, and how.
func pipelineSummary(fields []catalog.ManifestField, text func(string) string) i18n.Cat {
	var b i18n.Cat
	switch driver := text("source.driver"); driver {
	case "from.HTTP":
		b = append(b, i18n.M("summary.read.http", "what", joinWords(" ", text("source.method"), text("source.url"))))
	case "from.Files":
		b = append(b, i18n.M("summary.read.files", "path", text("source.path")))
	case "postgres.Query", "mysql.Query":
		engine := map[string]string{"postgres.Query": "Postgres", "mysql.Query": "MySQL"}[driver]
		b = append(b, i18n.M("summary.read.query", "engine", engine))
		if db := text("source.database"); db != "" {
			b = append(b, sp(), i18n.M("summary.read.database", "db", db))
		}
		if host := text("source.host"); host != "" {
			b = append(b, sp(), i18n.M("summary.read.host", "host", host))
		}
	case "from.Many":
		n := 0
		for _, f := range fields {
			if strings.HasPrefix(f.Path, "source.sources.") && strings.HasSuffix(f.Path, ".driver") {
				n++
			}
		}
		b = append(b, i18n.MN("summary.read.many", n))
	default:
		b = append(b, i18n.M("summary.read.other", "driver", driver))
	}

	transformers := 0
	for _, f := range fields {
		if strings.HasPrefix(f.Path, "transform.") && strings.HasSuffix(f.Path, ".map") {
			if list, ok := f.Value.([]any); ok {
				transformers += len(list)
			}
		}
	}
	if transformers > 0 {
		b = append(b, comma(), i18n.MN("summary.applies", transformers))
	}
	for _, f := range fields {
		if strings.HasPrefix(f.Path, "transform.") && strings.HasSuffix(f.Path, ".aggregate.by") {
			b = append(b, comma(), i18n.M("summary.groups", "by", valueText(f.Value)))
		}
	}

	target := text("target.driver")
	b = append(b, sp())
	switch target {
	case "bigquery.Table":
		b = append(b, i18n.M("summary.write.bigquery", "table", joinWords(".", text("target.project"), text("target.dataset"), text("target.table"))))
	case "postgres.Table", "redshift.Table":
		engine := map[string]string{"postgres.Table": "Postgres", "redshift.Table": "Redshift"}[target]
		b = append(b, i18n.M("summary.write.table", "engine", engine, "table", joinWords(".", text("target.schema"), text("target.table"))))
	case "mysql.Table":
		b = append(b, i18n.M("summary.write.table", "engine", "MySQL", "table", joinWords(".", text("target.database"), text("target.table"))))
	case "pubsub.Topic":
		b = append(b, i18n.M("summary.write.topic", "topic", joinWords("/", text("target.project"), text("target.topic"))))
	case "to.Files":
		b = append(b, i18n.M("summary.write.files", "path", text("target.path")))
	default:
		b = append(b, i18n.M("summary.write.other", "driver", target))
		if t := text("target.target"); t != "" {
			b = append(b, " ("+t+")")
		}
	}

	switch {
	case target == "pubsub.Topic":
		// A message is published, not written into a table: neither word fits.
	case text("target.dedup") == "merge":
		b = append(b, comma(), i18n.M("summary.pipeline.merge"))
	default:
		b = append(b, comma(), i18n.M("summary.pipeline.append"))
	}
	return append(b, stop())
}

// The punctuation that joins the pieces of a summary is the reader's language's, not English's: a space,
// a comma, a semicolon and a full stop are catalog messages (summary.sep.*), so Japanese says "、" and "。"
// and writes no space between clauses.
func sp() i18n.Msg    { return i18n.M("summary.sep.space") }
func comma() i18n.Msg { return i18n.M("summary.sep.comma") }
func semi() i18n.Msg  { return i18n.M("summary.sep.semicolon") }
func stop() i18n.Msg  { return i18n.M("summary.sep.stop") }
