package pages

import (
	"slices"
	"strings"

	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/infrastructure/sqlserve"
)

// SQLView is the workbench.
//
// A SCREEN OF ITS OWN, AND THAT IS THE POINT. `/data` answers "what did the
// pipelines land, and is it on time"; this answers "what is in it". The
// second question is not asked one table at a time, which is what a tab on a
// destination page forces.
type SQLView struct {
	// Targets are the destinations a SELECT could name, from the catalog.
	// A bucket has no columns, and offering one here is the same mistake as
	// drawing a Query tab on it.
	Targets []string

	// Target is the one chosen, Statement what is in the box -- ECHOED BACK,
	// because a query refused for a typo with the box emptied is a query
	// somebody has to type again.
	Target    string
	Statement string
	// Preloaded says Statement is a table's first look from a link, not what
	// somebody ran: the island opens it in a tab of its own rather than over
	// the one somebody was writing in.
	Preloaded bool

	Result *sqlserve.Result
	// Refused is the service declining the statement, in its own words; the
	// page titles it (screen-sql T3). ErrKey is a failure the console words
	// itself, with ErrArg filling it; Err any other failure, as it came.
	Refused        string
	ErrKey, ErrArg string
	Err            string

	// Tree is what the chosen connection holds, grouped by schema. Empty
	// when the warehouse cannot say -- the capability is optional and a
	// workbench without a tree still runs queries.
	Tree []Schema

	// Writable says this connection accepts statements that CHANGE data,
	// as the service reported it. The console never decides this and never
	// reads the SQL to guess at it.
	Writable bool

	// TreeCut says the service cut the listing, which has to be drawn: a
	// browser silently showing half a warehouse would have somebody conclude
	// a table does not exist.
	TreeCut bool

	// Open is the one relation whose columns are drawn, as `schema.name`,
	// and empty when none is. ONE, because CHECKPOINT D made columns lazy:
	// a tree that drew every relation's would be the project-wide COLUMNS
	// query it refused, assembled one request at a time.
	Open string

	// Cols is what Open holds. Empty when the warehouse cannot say, which
	// is not an error -- Describer is optional.
	Cols []sqlserve.Column
}

// Schema is one group in the tree.
type Schema struct {
	Name   string
	Tables []string
}

// Warehouse is one connection, and one destination that reaches it.
type Warehouse struct {
	// Name is `scheme://connection`, which is what identifies a connection.
	Name string

	// Target is one whole destination on it, because that is what every
	// request carries.
	Target string
}

// Warehouses groups the catalog's destinations by the connection they live on.
//
// A TARGET IS NOT A CONNECTION, and the tree is about connections. The
// catalog lists `bigquery://acme/bronze/orders` and
// `bigquery://acme/silver/daily` as two destinations, and `serve` resolves
// BOTH to one connection: `sql/internal/serve/target.go` reads a target as
// `scheme://connection/schema/table` and opens on the scheme and the FIRST
// segment -- `/v1/query` never looks at the rest. A tree with one root per
// target would draw the same warehouse twice, with identical contents under
// each.
//
// THE SPLIT IS REPEATED HERE AND THAT IS A COST, stated rather than hidden.
// The engine cannot import that parser: it holds no warehouse code, and
// `engine-weight.sh` is what enforces it. What keeps the repetition honest is
// that this grouping is only a LABEL -- every request still carries a whole
// target, which `serve` parses itself -- so a grouping that got it wrong
// would draw a wrong heading and never wrong data.
func Warehouses(targets []string) []Warehouse {
	var out []Warehouse
	at := map[string]bool{}
	for _, t := range targets {
		scheme, rest, ok := strings.Cut(t, "://")
		if !ok {
			continue
		}
		name := scheme + "://" + rest
		if first, _, ok := strings.Cut(rest, "/"); ok {
			name = scheme + "://" + first
		}
		if at[name] {
			continue
		}
		at[name] = true
		out = append(out, Warehouse{Name: name, Target: t})
	}
	return out
}

// Branch is one connection's listing, which the page and the fragment
// endpoint both draw.
//
// ONE DEFINITION, TWO DOORS. The island asks for a connection's objects over
// `/api/sql/objects` and the first page view gets the same markup from the
// template; two pieces of markup for one tree is how the classes, the ARIA
// and the "this warehouse does not say" drift apart.
type Branch struct {
	Target string
	Tree   []Schema
	Cut    bool

	// Open is the relation whose columns are drawn on the server path, and
	// empty on the fragment -- the island opens its own.
	Open string
	Cols []sqlserve.Column
}

// Warehouses is the picker's choices: one per connection.
func (v SQLView) Warehouses() []Warehouse { return Warehouses(v.Targets) }

// queryableSchemes are the warehouses `brevis-sql serve` has a dialect for
// (`sql/internal/dialect`). The catalog lists Redshift and MySQL tables too;
// the picker offers them disabled, with the reason, rather than a tree that
// answers nothing.
var queryableSchemes = map[string]bool{"bigquery": true, "postgres": true}

// Queryable says this console can read the destination.
func Queryable(target string) bool {
	scheme, _, ok := strings.Cut(target, "://")
	return ok && queryableSchemes[scheme]
}

// FirstQueryable is the destination the workbench opens on when it was given
// none: the first one it can read.
func (v SQLView) FirstQueryable() string {
	for _, t := range v.Targets {
		if Queryable(t) {
			return t
		}
	}
	return ""
}

// Unreadable is the kinds of warehouse in the catalog this console cannot
// read, as their makers write them, for the sentence under the picker.
func (v SQLView) Unreadable() []string {
	var out []string
	for _, w := range v.Warehouses() {
		scheme, _, _ := strings.Cut(w.Name, "://")
		if !queryableSchemes[scheme] && !slices.Contains(out, scheme) {
			out = append(out, scheme)
		}
	}
	return out
}

// SelectFor is a table's first look: every column, a hundred rows, the table
// named as a build names it -- `schema.table`, with no project or database,
// which is the connection's. "" for a destination that is not a table this
// console reads.
func SelectFor(target string) string {
	if !Queryable(target) {
		return ""
	}
	_, rest, _ := strings.Cut(target, "://")
	segs := strings.Split(rest, "/")
	if len(segs) != 3 || segs[1] == "" || segs[2] == "" {
		return ""
	}
	return selectOf(segs[1], segs[2])
}

func selectOf(schema, table string) string {
	return "SELECT *\nFROM " + schema + "." + table + "\nLIMIT 100;"
}

// BuildTree groups a listing by schema, in the order the service gave it.
//
// THE SERVICE ORDERS, NOT THIS. Both dialects sort in SQL, where the
// warehouse's own collation decides -- re-sorting here would be a second
// opinion about which of `Orders` and `orders` comes first.
func BuildTree(rels []sqlserve.Relation) []Schema {
	var out []Schema
	at := map[string]int{}
	for _, r := range rels {
		i, seen := at[r.Schema]
		if !seen {
			at[r.Schema] = len(out)
			out = append(out, Schema{Name: r.Schema})
			i = len(out) - 1
		}
		out[i].Tables = append(out[i].Tables, r.Name)
	}
	return out
}

// BuildSQL keeps only what can be queried.
func BuildSQL(entries []postgres.CatalogEntry) SQLView {
	v := SQLView{}
	seen := map[string]bool{}
	for _, e := range entries {
		if !catalog.IsRelation(e.Target) || seen[e.Target] {
			continue
		}
		seen[e.Target] = true
		v.Targets = append(v.Targets, e.Target)
	}
	return v
}

// Knows says the catalog lists this destination.
//
// ONE FUNCTION, TWO CALLERS, AND THAT IS THE POINT. `/sql`'s GET path had
// this loop written out and its POST path had nothing, so a destination the
// catalog did not list was refused in a link and accepted in a body. Two
// copies of a rule are two rules; the second one was missing for as long as
// the first one existed.
func (v SQLView) Knows(target string) bool {
	for _, t := range v.Targets {
		if t == target {
			return true
		}
	}
	return false
}

// ConnectionOf is the connection a destination lives on, as `serve` reads
// it: the first segment after the scheme -- what a missing `brevis.yaml`
// entry is named by.
func ConnectionOf(target string) string {
	_, rest, ok := strings.Cut(target, "://")
	if !ok {
		return target
	}
	first, _, _ := strings.Cut(rest, "/")
	return first
}
