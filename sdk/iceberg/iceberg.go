package iceberg

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AreteAcademy/brevis/sdk"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
)

// Table is an Apache Iceberg table as a destination: a Write appends the
// batch as Parquet and commits one snapshot, through whichever catalog
// holds the table's metadata pointer.
//
//	sdk.Load(ctx, rows, sdk.Target{
//	    To: iceberg.Table{
//	        Catalog:     lake,            // an iceberg.Catalog, opened once per process
//	        Namespace:   "bronze",
//	        Name:        "events",
//	        PartitionBy: "occurred_at",   // day(occurred_at) when the table is created
//	        CreateTable: &yes,
//	    },
//	    Schema: schema,                   // required to create
//	})
//
// Append only, in v1. Dedup is refused rather than imitated; a retried batch
// lands once through BatchKey. The table's shape is the consumer's
// declaration or the table's own, never inferred from a batch, and a
// divergence between the two is an error, never an ALTER.
type Table struct {
	// Catalog holds the table's metadata pointer. See Catalog.
	Catalog Catalog

	// Namespace is the table's namespace; a multi-level one is dotted,
	// "bronze.raw", and stays one segment of the locator.
	Namespace string

	// Name is the table's name.
	Name string

	// PartitionBy names the column a created table is partitioned by, as
	// day(column); on an existing table it has to match the table's spec.
	// Empty creates an unpartitioned table and makes no claim on an
	// existing one.
	PartitionBy string

	// CreateTable says whether a missing table is created, from Schema.
	// Tri-state like BigQuery's: nil lets the engine decide -- the first run
	// of a step creates, and so does the create_table parameter -- and an
	// explicit value wins over both.
	CreateTable *bool
}

var (
	_ sdk.Writer     = Table{}
	_ sdk.Locator    = Table{}
	_ sdk.Configured = Table{}
)

// Describe names the table the way Spark and Trino do, catalog.namespace.table.
func (t Table) Describe() string {
	return t.Catalog.Name + "." + t.Namespace + "." + t.Name
}

// Locate is the target the engine understands, without opening anything:
// iceberg://{catalog}/{namespace}/{table}, the dotted namespace one segment.
func (t Table) Locate() string {
	return "iceberg://" + t.Catalog.Name + "/" + t.Namespace + "/" + t.Name
}

// Configuration says what was chosen, for the console: the catalog, its
// type, the server without its userinfo, the warehouse, the table and the
// options. Never a DSN, a token, a credential or an IO key.
func (t Table) Configuration() []sdk.Setting {
	create := ""
	if t.CreateTable != nil {
		create = strconv.FormatBool(*t.CreateTable)
	}
	return keep(
		sdk.Setting{Key: "catalog", Value: t.Catalog.Name},
		sdk.Setting{Key: "catalog_type", Value: string(t.Catalog.Type)},
		sdk.Setting{Key: "uri", Value: withoutUserinfo(t.Catalog.URI)},
		sdk.Setting{Key: "warehouse", Value: t.Catalog.Warehouse},
		sdk.Setting{Key: "namespace", Value: t.Namespace},
		sdk.Setting{Key: "table", Value: t.Name},
		sdk.Setting{Key: "partition_by", Value: t.PartitionBy},
		sdk.Setting{Key: "create_table", Value: create},
	)
}

// Write refuses first, then loads the table -- creating it when asked and
// missing -- reconciles the batch against it, converts it in batches and
// appends them as one snapshot. The result says what happened: the rows,
// the Parquet bytes, the files, whether the table is new.
//
// Loaded on every Write, not cached: the table's current metadata is what
// the commit is conditioned on, and another writer may have moved it since
// the last one. See criterion 6 of the spec and commit.go.
func (t Table) Write(ctx context.Context, records []sdk.Envelope, opt sdk.WriteOptions) (*sdk.LoadResult, error) {
	start := time.Now()
	res := &sdk.LoadResult{Strategy: "append", Format: "parquet", Dedup: sdk.DedupNone}
	done := func(err error) (*sdk.LoadResult, error) {
		res.Duration = time.Since(start)
		return res, err
	}
	if err := t.check(); err != nil {
		return done(err)
	}
	if err := t.refuse(opt); err != nil {
		return done(err)
	}
	partitionBy := t.PartitionBy
	if partitionBy == "" {
		partitionBy = opt.PartitionBy
	}

	cat, err := t.Catalog.Open(ctx)
	if err != nil {
		return done(err)
	}
	tbl, err := cat.LoadTable(ctx, t.identifier())
	switch {
	case err == nil:
		if err := checkPartition(tbl.Spec(), tbl.Schema(), partitionBy); err != nil {
			return done(err)
		}
	case errors.Is(err, catalog.ErrNoSuchTable):
		tbl, res.TableCreated, err = t.create(ctx, cat, opt, records, partitionBy)
		if err != nil {
			return done(err)
		}
	default:
		return done(fmt.Errorf("iceberg: loading %s: %w", t.Describe(), err))
	}
	if len(records) == 0 {
		return done(nil)
	}

	schema, err := Reconcile(tbl.Schema(), records, opt)
	if err != nil {
		return done(err)
	}
	rdr, err := newReader(schema, records)
	if err != nil {
		return done(err)
	}
	defer rdr.Release()
	written, err := tbl.Append(ctx, rdr, nil)
	if err != nil {
		return done(fmt.Errorf("iceberg: appending to %s: %w", t.Describe(), err))
	}
	t.report(ctx, written, res, int64(len(records)))
	return done(nil)
}

// identifier is the catalog's name for the table: the namespace's levels,
// then the table.
func (t Table) identifier() table.Identifier {
	return table.Identifier(append(strings.Split(t.Namespace, "."), t.Name))
}

// create makes the table from the declaration, and the namespace if it is
// missing. Without a Schema it refuses listing the batch's columns, the way
// Postgres does: the message is the declaration the consumer has to write,
// not a guess at it. A table another writer created between our load and
// our create is loaded and reported as not ours.
func (t Table) create(ctx context.Context, cat catalog.Catalog, opt sdk.WriteOptions, records []sdk.Envelope, partitionBy string) (*table.Table, bool, error) {
	want, origin := t.createWanted(opt)
	if !want {
		return nil, false, fmt.Errorf("iceberg: the table %s does not exist, and CreateTable is false (%s). "+
			"Set CreateTable to true and declare Schema to create it, or create it in the catalog first", t.Describe(), origin)
	}
	if len(opt.Schema) == 0 {
		return nil, false, fmt.Errorf("iceberg: creating %s needs Schema, and none was declared: a table's types are not "+
			"inferred from a batch. The batch carries %s; declare each with its type, and ingestion_id and "+
			"ingestion_loaded_at as the chain writes them", t.Describe(), columnsOf(records))
	}
	sc, err := IcebergSchema(opt.Schema)
	if err != nil {
		return nil, false, err
	}
	spec, err := partitionSpec(partitionBy, sc)
	if err != nil {
		return nil, false, err
	}
	ident := t.identifier()
	ns := ident[:len(ident)-1]
	if err := cat.CreateNamespace(ctx, ns, nil); err != nil && !errors.Is(err, catalog.ErrNamespaceAlreadyExists) {
		return nil, false, fmt.Errorf("iceberg: creating namespace %s for %s: %w", t.Namespace, t.Describe(), err)
	}
	tbl, err := cat.CreateTable(ctx, ident, sc, catalog.WithPartitionSpec(&spec), catalog.WithProperties(tableProperties()))
	if errors.Is(err, catalog.ErrTableAlreadyExists) {
		tbl, err = cat.LoadTable(ctx, ident)
		if err != nil {
			return nil, false, fmt.Errorf("iceberg: loading %s after another writer created it: %w", t.Describe(), err)
		}
		return tbl, false, checkPartition(tbl.Spec(), tbl.Schema(), partitionBy)
	}
	if err != nil {
		return nil, false, fmt.Errorf("iceberg: creating %s: %w", t.Describe(), err)
	}
	return tbl, true, nil
}

// createWanted settles the tri-state the way bigquery.Table does, and says
// where the answer came from, for the refusal.
func (t Table) createWanted(opt sdk.WriteOptions) (bool, string) {
	if t.CreateTable != nil {
		return *t.CreateTable, "explicit"
	}
	switch {
	case opt.Run.First:
		return true, "the engine: first run of this step"
	case opt.Run.Params[sdk.ParamCreateTable] == "true":
		return true, "the engine: " + sdk.ParamCreateTable + "=true"
	}
	return false, "default"
}

// tableProperties is what a created table carries: format v2, which every
// reader of 2026 understands and v3's variant does not yet; zstd, the
// Parquet codec DuckDB, Spark and Trino all read and the one that keeps a
// landing table small; and commit retries on, so that the CAS form of a
// conflict is redone by iceberg-go without rewriting the files.
func tableProperties() ice.Properties {
	return ice.Properties{
		"format-version":            "2",
		table.ParquetCompressionKey: "zstd",
		table.CommitNumRetriesKey:   "4",
	}
}

// report fills the result from the snapshot the append produced: the rows
// and bytes from its summary, and the data files it added, read from its
// manifests. The files are best effort -- the commit has happened, and a
// result that fails over a listing would send the caller to retry a write
// that landed.
func (t Table) report(ctx context.Context, tbl *table.Table, res *sdk.LoadResult, n int64) {
	res.RowsLoaded = n
	snap := tbl.CurrentSnapshot()
	if snap == nil || snap.Summary == nil {
		return
	}
	if v, err := strconv.ParseInt(snap.Summary.Properties["added-records"], 10, 64); err == nil {
		res.RowsLoaded = v
	}
	if v, err := strconv.ParseInt(snap.Summary.Properties["added-files-size"], 10, 64); err == nil {
		res.BytesStaged = v
	}
	res.Objects = addedFiles(ctx, tbl, snap)
}

func addedFiles(ctx context.Context, tbl *table.Table, snap *table.Snapshot) []string {
	fs, err := tbl.FS(ctx)
	if err != nil {
		return nil
	}
	manifests, err := snap.Manifests(fs)
	if err != nil {
		return nil
	}
	var paths []string
	for _, m := range manifests {
		if m.SnapshotID() != snap.SnapshotID {
			continue
		}
		for e, err := range m.Entries(fs, true) {
			if err != nil {
				break
			}
			if e.Status() == ice.EntryStatusADDED {
				paths = append(paths, e.DataFile().FilePath())
			}
		}
	}
	sort.Strings(paths)
	return paths
}

// columnsOf lists the fields the batch carries with a value, sorted, for
// the refusal that asks for a Schema.
func columnsOf(records []sdk.Envelope) string {
	valued := valuedFields(records)
	names := make([]string, 0, len(valued))
	for k := range valued {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// keep is the settings that say something: a zero value is a setting nobody
// chose, and listing it would bury the ones they did.
func keep(settings ...sdk.Setting) []sdk.Setting {
	out := make([]sdk.Setting, 0, len(settings))
	for _, s := range settings {
		switch v := s.Value.(type) {
		case string:
			if v == "" {
				continue
			}
		case bool:
			if !v {
				continue
			}
		case nil:
			continue
		}
		out = append(out, s)
	}
	return out
}

// withoutUserinfo is the URI with any user:password stripped, for the
// console; an unparsable URI is shown as nothing rather than as it came.
func withoutUserinfo(uri string) string {
	if uri == "" {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	u.User = nil
	return u.String()
}

// reader is Records as the array.RecordReader iceberg-go's Append takes:
// pull-based over the push iterator, one batch alive at a time. The reader
// holds its own reference to the current batch until Next moves on, which
// is the Arrow contract -- a consumer that keeps a batch across Next calls
// Retains it, and iceberg-go's bin packer does.
var _ array.RecordReader = (*reader)(nil)

type reader struct {
	schema *arrow.Schema
	next   func() (arrow.RecordBatch, error, bool)
	stop   func()
	cur    arrow.RecordBatch
	err    error
	refs   atomic.Int64
}

func newReader(schema sdk.Schema, records []sdk.Envelope) (*reader, error) {
	as, err := ArrowSchema(schema)
	if err != nil {
		return nil, err
	}
	next, stop := iter.Pull2(Records(schema, records))
	r := &reader{schema: as, next: next, stop: stop}
	r.refs.Store(1)
	return r, nil
}

func (r *reader) Retain() { r.refs.Add(1) }

func (r *reader) Release() {
	if r.refs.Add(-1) == 0 {
		r.drop()
		r.stop()
	}
}

func (r *reader) Schema() *arrow.Schema { return r.schema }

func (r *reader) Next() bool {
	r.drop()
	if r.err != nil {
		return false
	}
	rec, err, ok := r.next()
	if !ok {
		return false
	}
	if err != nil {
		r.err = err
		return false
	}
	rec.Retain()
	r.cur = rec
	return true
}

func (r *reader) drop() {
	if r.cur != nil {
		r.cur.Release()
		r.cur = nil
	}
}

func (r *reader) RecordBatch() arrow.RecordBatch { return r.cur }

// Record is the deprecated name of RecordBatch, still in the interface.
func (r *reader) Record() arrow.RecordBatch { return r.cur }

func (r *reader) Err() error { return r.err }
