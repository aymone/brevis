package iceberg

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk"
	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/hadoop"
)

// events is the table the Table tests write: the landing columns plus the
// instant the rows are partitioned by.
var events = sdk.Schema{
	{Name: "event_id", Type: sdk.TypeString, Required: true},
	{Name: "occurred_at", Type: sdk.TypeTimestamp, Required: true},
	{Name: "amount", Type: sdk.TypeNumeric},
	{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
}

// onDisk is a catalog that needs no service: iceberg-go's filesystem catalog
// on a temporary directory, metadata and data files both, handed to the
// Table through Conn. What the Table does with it is exactly what it does
// with Postgres or a REST server, because the catalog interface is one.
func onDisk(t *testing.T) Catalog {
	t.Helper()
	cat, err := hadoop.NewCatalog("lake", "file://"+t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return Catalog{Name: "lake", Conn: cat}
}

func eventRows(n int, from int) []sdk.Envelope {
	out := make([]sdk.Envelope, n)
	for i := range out {
		k := from + i
		out[i] = row(fmt.Sprintf("e-%d", k), fields{
			"event_id":            fmt.Sprintf("e-%d", k),
			"occurred_at":         fmt.Sprintf("2026-11-%02dT10:00:00Z", k%28+1),
			"amount":              fmt.Sprintf("%d.50", k),
			"ingestion_loaded_at": loadedAt,
		})
	}
	return out
}

func yes() *bool { v := true; return &v }
func no() *bool  { v := false; return &v }

// THE FIRST WRITE CREATES THE TABLE AND THE SECOND APPENDS TO IT (criteria 1
// and 3): a v2 table, zstd Parquet, partitioned by day(occurred_at), in a
// namespace created on the way. The result says what happened -- the rows,
// the bytes, the files, whether the table is new -- and the second result
// says the table was already there.
func TestTheFirstWriteCreatesTheTableAndTheSecondAppends(t *testing.T) {
	ctx := context.Background()
	tbl := Table{Catalog: onDisk(t), Namespace: "bronze", Name: "events", PartitionBy: "occurred_at", CreateTable: yes()}
	opt := sdk.WriteOptions{Columns: events.Names(), Schema: events}

	res, err := tbl.Write(ctx, eventRows(3, 1), opt)
	if err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if !res.TableCreated || res.RowsLoaded != 3 || res.Strategy != "append" || res.Format != "parquet" {
		t.Errorf("first result: %+v, want 3 rows appended as parquet into a created table", res)
	}
	if res.BytesStaged <= 0 || len(res.Objects) == 0 {
		t.Errorf("first result: %d bytes, %d objects; want the Parquet written", res.BytesStaged, len(res.Objects))
	}
	for _, o := range res.Objects {
		if !strings.HasSuffix(o, ".parquet") || !strings.Contains(o, "/bronze") {
			t.Errorf("object %q is not a Parquet file under the namespace", o)
		}
	}

	got, err := tbl.Catalog.Conn.LoadTable(ctx, catalog.ToIdentifier("bronze", "events"))
	if err != nil {
		t.Fatalf("the created table does not load: %v", err)
	}
	if v := got.Metadata().Version(); v != 2 {
		t.Errorf("format version %d, want 2", v)
	}
	if c := got.Properties()["write.parquet.compression-codec"]; c != "zstd" {
		t.Errorf("compression %q, want zstd", c)
	}
	spec := got.Spec()
	if spec.NumFields() != 1 || spec.Field(0).Transform.String() != "day" || spec.Field(0).Name != "occurred_at_day" {
		t.Errorf("partition spec %s, want day(occurred_at)", spec.String())
	}
	if n := len(got.Schema().Fields()); n != len(events) {
		t.Errorf("%d columns, want %d", n, len(events))
	}

	res, err = tbl.Write(ctx, eventRows(2, 4), opt)
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if res.TableCreated || res.RowsLoaded != 2 {
		t.Errorf("second result: %+v, want 2 rows appended to an existing table", res)
	}
	got, _ = tbl.Catalog.Conn.LoadTable(ctx, catalog.ToIdentifier("bronze", "events"))
	if n := len(got.Metadata().Snapshots()); n != 2 {
		t.Errorf("%d snapshots, want one per Write", n)
	}
	if total := got.CurrentSnapshot().Summary.Properties["total-records"]; total != "5" {
		t.Errorf("total-records %q, want 5", total)
	}
}

// NO ROWS IS NOT AN ERROR AND NOT A SNAPSHOT: the table is created if asked,
// so a first run with nothing to send still leaves the table for the next
// one, and nothing is appended.
func TestAWriteWithNoRowsCreatesTheTableAndAppendsNothing(t *testing.T) {
	ctx := context.Background()
	tbl := Table{Catalog: onDisk(t), Namespace: "bronze", Name: "events", CreateTable: yes()}
	res, err := tbl.Write(ctx, nil, sdk.WriteOptions{Columns: events.Names(), Schema: events})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TableCreated || res.RowsLoaded != 0 || len(res.Objects) != 0 {
		t.Errorf("result %+v, want a created table and nothing written", res)
	}
	got, err := tbl.Catalog.Conn.LoadTable(ctx, catalog.ToIdentifier("bronze", "events"))
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentSnapshot() != nil {
		t.Error("an empty Write produced a snapshot")
	}
}

// WITHOUT A SCHEMA, CREATION IS REFUSED LISTING THE BATCH'S COLUMNS, the way
// Postgres does: the message is the declaration the consumer has to write,
// not a guess at it.
func TestWithoutASchemaCreationIsRefusedListingTheBatchColumns(t *testing.T) {
	tbl := Table{Catalog: onDisk(t), Namespace: "bronze", Name: "events", CreateTable: yes()}
	_, err := tbl.Write(context.Background(), rows(
		fields{"event_id": "a", "amount": "1", "ingestion_loaded_at": loadedAt},
	), sdk.WriteOptions{})
	if err == nil {
		t.Fatal("created a table from a batch")
	}
	for _, want := range []string{"Schema", "amount, event_id, ingestion_loaded_at", "lake.bronze.events"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %s", err, want)
		}
	}
}

// A TABLE THAT DOES NOT EXIST AND MAY NOT BE CREATED IS A REFUSAL NAMING THE
// TABLE AND THE OPTION; and the tri-state resolves the way BigQuery's does:
// explicit first, then the engine's first run or its parameter.
func TestAMissingTableWithoutCreateTableIsRefused(t *testing.T) {
	ctx := context.Background()
	opt := sdk.WriteOptions{Columns: events.Names(), Schema: events}
	tbl := Table{Catalog: onDisk(t), Namespace: "bronze", Name: "events", CreateTable: no()}
	_, err := tbl.Write(ctx, eventRows(1, 1), opt)
	if err == nil || !strings.Contains(err.Error(), "lake.bronze.events") || !strings.Contains(err.Error(), "CreateTable") {
		t.Fatalf("missing table, CreateTable false: %v, want a refusal naming both", err)
	}

	tbl.CreateTable = nil
	opt.Run.First = true
	if _, err := tbl.Write(ctx, eventRows(1, 1), opt); err != nil {
		t.Errorf("the engine's first run should create: %v", err)
	}
}

// DEDUP, A DEFAULT, A UNIQUE AND TWO PARTITION COLUMNS ARE REFUSED BEFORE
// ANYTHING IS OPENED
// (criterion 5), each naming the option and what to do. The catalog here
// points at a closed port: a refusal that got as far as the network would
// say "connection refused" instead.
func TestUnsupportedOptionsAreRefusedBeforeTheCatalogOpens(t *testing.T) {
	tbl := Table{Catalog: Catalog{Name: "lake", Type: REST, URI: "http://127.0.0.1:1"}, Namespace: "bronze", Name: "events"}
	cases := []struct {
		name string
		opt  sdk.WriteOptions
		want []string
	}{
		{"dedup", sdk.WriteOptions{Dedup: sdk.DedupMerge}, []string{"Dedup", "BatchKey"}},
		{"default", sdk.WriteOptions{Schema: sdk.Schema{{Name: "loaded", Type: sdk.TypeTimestamp, Default: "now"}}}, []string{`"loaded"`, "Default"}},
		{"unique", sdk.WriteOptions{Schema: sdk.Schema{{Name: "event_id", Type: sdk.TypeString, Unique: true}}}, []string{`"event_id"`, "Unique"}},
		{"two partition columns", sdk.WriteOptions{PartitionBy: "ingestion_loaded_at"}, []string{"PartitionBy", "occurred_at", "ingestion_loaded_at"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tb := tbl
			if c.name == "two partition columns" {
				tb.PartitionBy = "occurred_at"
			}
			_, err := tb.Write(context.Background(), eventRows(1, 1), c.opt)
			if err == nil {
				t.Fatal("accepted")
			}
			if strings.Contains(err.Error(), "connection refused") {
				t.Errorf("the refusal reached the network: %v", err)
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

// PARTITIONBY MUST MATCH AN EXISTING TABLE (criterion 5, third refusal): a
// table partitioned by day(occurred_at) refuses a PartitionBy that names
// another column, saying what the table has. No PartitionBy means no claim,
// and the write goes through. And the spec itself is a pure function: the
// column has to exist and has to be an instant or a date, because day() of
// a string is not a thing.
func TestPartitionByMustMatchAnExistingTable(t *testing.T) {
	ctx := context.Background()
	lake := onDisk(t)
	opt := sdk.WriteOptions{Columns: events.Names(), Schema: events}
	if _, err := (Table{Catalog: lake, Namespace: "bronze", Name: "events", PartitionBy: "occurred_at", CreateTable: yes()}).Write(ctx, eventRows(1, 1), opt); err != nil {
		t.Fatal(err)
	}
	_, err := (Table{Catalog: lake, Namespace: "bronze", Name: "events", PartitionBy: "ingestion_loaded_at"}).Write(ctx, eventRows(1, 2), opt)
	if err == nil || !strings.Contains(err.Error(), "PartitionBy") || !strings.Contains(err.Error(), "day(occurred_at)") {
		t.Errorf("another PartitionBy on an existing table: %v, want a refusal saying what the table has", err)
	}
	if _, err := (Table{Catalog: lake, Namespace: "bronze", Name: "events"}).Write(ctx, eventRows(1, 3), opt); err != nil {
		t.Errorf("no PartitionBy on an existing table: %v, want the write to go through", err)
	}

	sc, _ := IcebergSchema(events)
	if _, err := partitionSpec("host", sc); err == nil || !strings.Contains(err.Error(), `"host"`) {
		t.Errorf("a column the schema lacks: %v", err)
	}
	if _, err := partitionSpec("event_id", sc); err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("day() of a string: %v", err)
	}
	spec, err := partitionSpec("", sc)
	if err != nil || !spec.IsUnpartitioned() {
		t.Errorf("no column: %v, %s; want unpartitioned", err, spec.String())
	}
}

// LOCATE IS THE TARGET THE ENGINE UNDERSTANDS, WITHOUT OPENING ANYTHING, and
// a dotted namespace stays one segment (criterion 9). CONFIGURATION says
// what was chosen and never a DSN, a token, a credential or a key: it goes
// to the console, and the console is not where secrets live.
func TestLocateDescribeAndConfigurationCarryNoSecret(t *testing.T) {
	tbl := Table{
		Catalog: Catalog{Name: "lake", Type: SQL, DSN: "postgres://brevis:dsn-secret@db/lake", Token: "token-secret",
			Credential: "id:cred-secret", URI: "https://polaris.example.com/api", Warehouse: "s3://lake/warehouse",
			IO: map[string]string{"s3.access-key-id": "key-id", "s3.secret-access-key": "s3-secret", "s3.endpoint": "http://seaweedfs:8333"}},
		Namespace: "bronze.raw", Name: "events", PartitionBy: "occurred_at", CreateTable: yes(),
	}
	if got := tbl.Locate(); got != "iceberg://lake/bronze.raw/events" {
		t.Errorf("Locate() = %q", got)
	}
	if got := tbl.Describe(); got != "lake.bronze.raw.events" {
		t.Errorf("Describe() = %q", got)
	}
	cfg := tbl.Configuration()
	keys := map[string]any{}
	for _, s := range cfg {
		keys[s.Key] = s.Value
		for _, secret := range []string{"dsn-secret", "token-secret", "cred-secret", "s3-secret", "key-id"} {
			if strings.Contains(fmt.Sprint(s.Value), secret) {
				t.Errorf("setting %s carries %s", s.Key, secret)
			}
		}
	}
	for _, want := range []string{"catalog", "catalog_type", "warehouse", "namespace", "table", "partition_by", "create_table"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("Configuration lacks %s: %v", want, keys)
		}
	}
	if keys["catalog"] != "lake" || keys["namespace"] != "bronze.raw" || keys["table"] != "events" {
		t.Errorf("Configuration names %v", keys)
	}
}

var (
	_ sdk.Writer     = Table{}
	_ sdk.Locator    = Table{}
	_ sdk.Configured = Table{}
	_                = ice.UnpartitionedSpec
)
