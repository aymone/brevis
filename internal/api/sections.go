package api

import "brevis.sh/brevis/web/i18n"

// section is one place in the console, with the group it sits in: what the
// command palette lists under Sections. Name and Group are in the reader's
// language (nav.<key>, nav.group.<name> in the catalog).
//
// ONE LIST FOR TWO READERS: the sidebar in web/spa/src/shell/nav.ts draws the
// same places in the same groups. SQL is in both or in neither: a section that
// answers "no SQL service is configured" leads nowhere, and the palette, like
// the sidebar, is where a reader trusts that what is listed exists.
type section struct {
	Group, Name i18n.Msg
	Href, Icon  string
}

func sections(hasSQL bool) []section {
	place := func(group i18n.Msg, href, key, icon string) section {
		return section{Group: group, Href: href, Name: i18n.M("nav." + key), Icon: icon}
	}
	monitor, orchestrate := i18n.M("nav.group.monitor"), i18n.M("nav.group.orchestrate")
	ingest, data := i18n.M("nav.group.ingest"), i18n.M("nav.group.data")
	out := []section{
		place(monitor, "/", "overview", "layout-grid"),
		place(monitor, "/runs", "runs", "history"),
		place(orchestrate, "/workflows", "workflows", "workflow"),
		place(ingest, "/ingest/streams", "streams", "cloud"),
		place(ingest, "/ingest/loads", "loads", "download"),
		place(data, "/data", "catalog", "database"),
		place(data, "/data/endpoints", "endpoints", "plug"),
	}
	if hasSQL {
		out = append(out, place(data, "/sql", "sql", "terminal"))
	}
	return out
}
