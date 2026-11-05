package api

import (
	"context"

	"brevis.sh/brevis/web/i18n"
)

// section is one place in the console, with the group it sits in: what the
// command palette lists under Sections. Name and Group are in the reader's
// language (nav.<key>, nav.group.<name> in the catalog).
//
// ONE LIST FOR TWO READERS: the sidebar in web/spa/src/shell/nav.ts draws the
// same places in the same groups. SQL is in both or in neither: a section that
// answers "no SQL service is configured" leads nowhere, and the palette, like
// the sidebar, is where a reader trusts that what is listed exists.
type section struct {
	Group, Href, Name, Icon string
}

func sections(ctx context.Context, hasSQL bool) []section {
	place := func(group, href, key, icon string) section {
		return section{Group: group, Href: href, Name: i18n.T(ctx, "nav."+key), Icon: icon}
	}
	monitor, orchestrate := i18n.T(ctx, "nav.group.monitor"), i18n.T(ctx, "nav.group.orchestrate")
	ingest, data := i18n.T(ctx, "nav.group.ingest"), i18n.T(ctx, "nav.group.data")
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
