package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/infrastructure/sqlserve"
	"brevis.sh/brevis/web/pages"
)

// The workbench's JSON doors, for the React screen. They sit beside the HTML
// fragments of sql_api.go rather than replace them: the old page keeps working
// until the cutover, and the rules (catalog check, statement ceiling, row
// limit) are the same functions, not copies -- BuildSQL/Knows, queryRows and
// statementCeiling.
//
// A failure is {"error": "..."} with a status, plus "kind" where the screen
// words it itself.

func (u *UI) registerSQLJSON(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sql/state", u.sqlState)
	mux.HandleFunc("GET /api/sql/tree", u.sqlTree)
	mux.HandleFunc("GET /api/sql/describe", u.sqlDescribe)
	mux.HandleFunc("POST /api/sql/run", u.sqlRun)
	mux.HandleFunc("POST /api/sql/cost", u.sqlCost)
}

type sqlConnection struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	Queryable bool   `json:"queryable"`
}

type sqlSchema struct {
	Name   string   `json:"name"`
	Tables []string `json:"tables"`
}

type sqlTreeView struct {
	Schemas  []sqlSchema `json:"schemas"`
	Cut      bool        `json:"cut"`
	Writable bool        `json:"writable"`
}

type sqlStateView struct {
	Connections []sqlConnection `json:"connections"`
	Unreadable  []string        `json:"unreadable"`
	Target      string          `json:"target"`
	// Statement is a table's first look from a link (?target=), never read
	// from the link itself: SelectFor builds it from a destination the catalog
	// knows.
	Statement string      `json:"statement"`
	Preloaded bool        `json:"preloaded"`
	Tree      sqlTreeView `json:"tree"`
	// History is empty by design: the server keeps none, the screen keeps its
	// own in the browser.
	History []string `json:"history"`
}

// sqlView is the guard every JSON door shares: a configured service and the
// catalog's view of what may be queried. ok is false once it has answered.
func (u *UI) sqlView(w http.ResponseWriter, r *http.Request) (pages.SQLView, bool) {
	if !u.preview.Configured() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no SQL service is configured"})
		return pages.SQLView{}, false
	}
	var entries []postgres.CatalogEntry
	if u.catalog != nil {
		var err error
		if entries, err = u.catalog.Catalog(r.Context()); err != nil {
			u.log.Error("reading the catalog for the workbench", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the catalog is unavailable"})
			return pages.SQLView{}, false
		}
	}
	return pages.BuildSQL(entries), true
}

// sqlKnownTarget is sqlView plus the "known destination" rule, answered with
// the same 404 for "no such" and "not one you may ask about".
func (u *UI) sqlKnownTarget(w http.ResponseWriter, r *http.Request, target string) (string, bool) {
	v, ok := u.sqlView(w, r)
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" || !v.Knows(target) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not a destination this console knows"})
		return "", false
	}
	return target, true
}

// treeOf lists one connection. A warehouse that cannot say is an empty tree,
// not an error (Relations is an optional capability).
func (u *UI) treeOf(ctx context.Context, target string) sqlTreeView {
	out := sqlTreeView{Schemas: []sqlSchema{}}
	objs, err := u.preview.Objects(ctx, target)
	if err != nil {
		return out
	}
	for _, s := range pages.BuildTree(objs.Relations) {
		out.Schemas = append(out.Schemas, sqlSchema{Name: s.Name, Tables: s.Tables})
	}
	out.Cut, out.Writable = objs.Truncated, objs.Writable
	return out
}

func (u *UI) sqlState(w http.ResponseWriter, r *http.Request) {
	v, ok := u.sqlView(w, r)
	if !ok {
		return
	}
	st := sqlStateView{Connections: []sqlConnection{}, Unreadable: []string{}, History: []string{},
		Tree: sqlTreeView{Schemas: []sqlSchema{}}}
	for _, wh := range v.Warehouses() {
		st.Connections = append(st.Connections, sqlConnection{Name: wh.Name, Target: wh.Target, Queryable: pages.Queryable(wh.Target)})
	}
	st.Unreadable = append(st.Unreadable, v.Unreadable()...)

	q := r.URL.Query()
	if want := q.Get("target"); want != "" && v.Knows(want) && pages.Queryable(want) {
		st.Target = want
		st.Statement, st.Preloaded = pages.SelectFor(want), pages.SelectFor(want) != ""
	}
	if want := q.Get("connection"); want != "" && v.Knows(want) && pages.Queryable(want) {
		st.Target = want
	}
	if st.Target == "" {
		st.Target = v.FirstQueryable()
	}
	if st.Target != "" {
		st.Tree = u.treeOf(r.Context(), st.Target)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, st)
}

func (u *UI) sqlTree(w http.ResponseWriter, r *http.Request) {
	target, ok := u.sqlKnownTarget(w, r, r.URL.Query().Get("target"))
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, u.treeOf(r.Context(), target))
}

func (u *UI) sqlDescribe(w http.ResponseWriter, r *http.Request) {
	target, ok := u.sqlKnownTarget(w, r, r.URL.Query().Get("target"))
	if !ok {
		return
	}
	schema, name := r.URL.Query().Get("schema"), r.URL.Query().Get("name")
	if schema == "" || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schema and name are required"})
		return
	}
	// An undescribable relation is an empty list, as in the HTML door.
	cols := []sqlserve.Column{}
	if got, err := u.preview.Columns(r.Context(), target, schema, name); err == nil && got != nil {
		cols = got
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"columns": cols})
}

type sqlBody struct {
	Target string `json:"target"`
	Q      string `json:"q"`
}

// sqlBodyOf reads {target, q}, bounded by the same ceiling as the page.
func sqlBodyOf(w http.ResponseWriter, r *http.Request) (sqlBody, bool) {
	var b sqlBody
	r.Body = http.MaxBytesReader(w, r.Body, statementCeiling)
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "that statement is too long to run", "kind": "too_long"})
		return b, false
	}
	b.Q = strings.TrimSpace(b.Q)
	return b, true
}

// sqlRun is the POST of the page, as JSON. The statement travels in the body
// and never in a URL.
func (u *UI) sqlRun(w http.ResponseWriter, r *http.Request) {
	b, ok := sqlBodyOf(w, r)
	if !ok {
		return
	}
	v, ok := u.sqlView(w, r)
	if !ok {
		return
	}
	// The catalog is checked on the body too: the service answers differently
	// for a connection it holds, which would let a guess probe its config.
	if b.Target == "" || !v.Knows(b.Target) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not a destination this console knows", "kind": "unknown"})
		return
	}
	if b.Q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "there is no statement to run", "kind": "empty"})
		return
	}
	res, err := u.preview.Query(r.Context(), b.Target, b.Q, queryRows)
	if err != nil {
		var refused *sqlserve.Refused
		switch {
		case errors.Is(err, sqlserve.ErrNoConnection):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": err.Error(), "kind": "no_connection", "connection": pages.ConnectionOf(b.Target)})
		case errors.As(err, &refused):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": refused.Why, "kind": "refused"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "kind": "failed"})
		}
		return
	}
	if res.Columns == nil {
		res.Columns = []string{}
	}
	if res.Rows == nil {
		res.Rows = [][]any{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

// sqlCost prices a statement. Silence (204) is an answer, as in sqlEstimate.
func (u *UI) sqlCost(w http.ResponseWriter, r *http.Request) {
	b, ok := sqlBodyOf(w, r)
	if !ok {
		return
	}
	target, ok := u.sqlKnownTarget(w, r, b.Target)
	if !ok {
		return
	}
	if b.Q == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	price, err := u.preview.Estimate(r.Context(), target, b.Q)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": price.State, "bytes": price.Bytes, "why": price.Why})
}
