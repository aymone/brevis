package iceberg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AreteAcademy/brevis/sdk"
	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	iceio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud/s3" // the S3 backend, a consumer's blank import -- here the tests are the consumer
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// The integration tests: the destination against the catalogs and the
// storage it will meet, each behind its variable and SKIPPED without it,
// the way every integration test in this SDK is. CI sets the variables and
// then checks that the tests ran, because a skip prints green.
//
//	docker compose -f docker-compose.drivers.yml up -d postgres minio iceberg-rest
//	BREVIS_ICEBERG_TEST_PG_DSN=postgres://brevis:brevis@localhost:55432/brevis_it \
//	BREVIS_ICEBERG_TEST_S3_ENDPOINT=http://localhost:9000 \
//	BREVIS_ICEBERG_TEST_REST_URI=http://localhost:8181 \
//	AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=brevis AWS_SECRET_ACCESS_KEY=brevis-secret \
//	go test ./... -run Integration -count=1 -v
//
// BREVIS_ICEBERG_TEST_ORACLE names a file the SQL test writes for the
// DuckDB oracle in CI: the table's current metadata location and what the
// scan has to find there.

func envOrSkip(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("%s is not set; see the comment at the top of integration_test.go", name)
	}
	return v
}

// storage is the S3 the tests write to: the endpoint from its variable,
// the credentials from the AWS ones every S3 client reads.
type storage struct {
	endpoint, region, key, secret, bucket string
}

func storageOf(t *testing.T) storage {
	t.Helper()
	s := storage{
		endpoint: envOrSkip(t, "BREVIS_ICEBERG_TEST_S3_ENDPOINT"),
		region:   envOr("AWS_REGION", "us-east-1"),
		key:      envOr("AWS_ACCESS_KEY_ID", "brevis"),
		secret:   envOr("AWS_SECRET_ACCESS_KEY", "brevis-secret"),
		bucket:   envOr("BREVIS_ICEBERG_TEST_S3_BUCKET", "lake"),
	}
	s.ensureBucket(t)
	return s
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// io is the Catalog.IO this storage needs, in iceberg-go's own keys.
func (s storage) io() map[string]string {
	return map[string]string{
		iceio.S3EndpointURL:            s.endpoint,
		iceio.S3Region:                 s.region,
		iceio.S3AccessKeyID:            s.key,
		iceio.S3SecretAccessKey:        s.secret,
		iceio.S3ForceVirtualAddressing: "false",
	}
}

func (s storage) warehouse() string { return "s3://" + s.bucket + "/warehouse" }

// ensureBucket creates the bucket: iceberg-go writes into one and never
// creates it, and a fresh s3mock or SeaweedFS has none.
func (s storage) ensureBucket(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(s.region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.key, s.secret, "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.endpoint)
		o.UsePathStyle = true
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") && !strings.Contains(err.Error(), "BucketAlreadyExists") {
		t.Fatalf("creating bucket %s at %s: %v", s.bucket, s.endpoint, err)
	}
}

// lakeEvents is the integration table: the landing columns plus a quantity
// whose sum the DuckDB oracle checks, because a sum proves the values and
// not only the row count.
var lakeEvents = sdk.Schema{
	{Name: "event_id", Type: sdk.TypeString, Required: true},
	{Name: "occurred_at", Type: sdk.TypeTimestamp, Required: true},
	{Name: "quantity", Type: sdk.TypeInt64},
	{Name: "amount", Type: sdk.TypeNumeric},
	{Name: "ingestion_loaded_at", Type: sdk.TypeTimestamp, Required: true},
}

// lakeRows are n rows from `from`, quantity = the row's number, so that
// sum(quantity) over any set of batches is known in advance.
func lakeRows(n, from int) (rows []sdk.Envelope, sum int64) {
	rows = make([]sdk.Envelope, n)
	for i := range rows {
		k := from + i
		sum += int64(k)
		rows[i] = row(fmt.Sprintf("e-%d", k), fields{
			"event_id":            fmt.Sprintf("e-%d", k),
			"occurred_at":         fmt.Sprintf("2026-11-%02dT10:00:00Z", k%28+1),
			"quantity":            k,
			"amount":              fmt.Sprintf("%d.25", k),
			"ingestion_loaded_at": loadedAt,
		})
	}
	return rows, sum
}

// outcome is what a run of the shared scenario leaves for the oracle.
type outcome struct {
	Metadata    string `json:"metadata"`
	Rows        int64  `json:"rows"`
	SumQuantity int64  `json:"sum_quantity"`
}

// exercise runs the scenario the spec's criteria name against one catalog:
// the first Write creates, a replay of its key lands nothing, a second
// batch appends, and two more writers through connections of their own
// land every batch. `open(0)` is the main writer's Catalog and `open(n)`
// the n-th extra writer's, each a connection of its own to the SAME
// catalog -- what two processes are.
func exercise(t *testing.T, open func(writer int) Catalog) outcome {
	t.Helper()
	ctx := context.Background()
	ns := fmt.Sprintf("it_%d", time.Now().UnixNano())
	tbl := Table{Catalog: open(0), Namespace: ns, Name: "events", PartitionBy: "occurred_at", CreateTable: yes()}
	t.Cleanup(func() {
		cat, err := tbl.Catalog.Open(ctx)
		if err != nil {
			return
		}
		_ = cat.DropTable(ctx, catalog.ToIdentifier(ns, "events"))
		_ = cat.DropNamespace(ctx, catalog.ToIdentifier(ns))
	})
	opt := func(key string) sdk.WriteOptions {
		return sdk.WriteOptions{Columns: lakeEvents.Names(), Schema: lakeEvents, BatchKey: key}
	}
	var total, sum int64

	rows, s := lakeRows(100, 0)
	res, err := tbl.Write(ctx, rows, opt("b-1"))
	if err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if !res.TableCreated || res.RowsLoaded != 100 || res.Strategy != "append" || len(res.Objects) == 0 {
		t.Fatalf("first result %+v, want 100 rows into a created table with its files listed", res)
	}
	for _, o := range res.Objects {
		if !strings.HasPrefix(o, "s3://") || !strings.HasSuffix(o, ".parquet") {
			t.Errorf("object %q, want a Parquet file on S3", o)
		}
	}
	total, sum = total+100, sum+s

	replay, err := tbl.Write(ctx, rows[:7], opt("b-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Strategy != "already-committed" || replay.RowsLoaded != 100 || len(replay.Objects) != 0 {
		t.Errorf("replay result %+v, want already-committed with the 100 rows of the first write", replay)
	}

	rows, s = lakeRows(50, 1000)
	if res, err = tbl.Write(ctx, rows, opt("b-2")); err != nil || res.TableCreated || res.RowsLoaded != 50 {
		t.Fatalf("second batch: %+v, %v", res, err)
	}
	total, sum = total+50, sum+s

	// Two writers, each through its own catalog connection, on one table.
	const writers, each, per = 2, 4, 25
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := []error{}
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			mine := Table{Catalog: open(w + 1), Namespace: ns, Name: "events"}
			for i := 0; i < each; i++ {
				rows, s := lakeRows(per, 10000+w*1000+i*per)
				if _, err := mine.Write(ctx, rows, opt(fmt.Sprintf("w%d/%d", w, i))); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("writer %d batch %d: %w", w, i, err))
					mu.Unlock()
					continue
				}
				mu.Lock()
				total, sum = total+per, sum+s
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	for _, err := range errs {
		t.Error(err)
	}

	cat, err := tbl.Catalog.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cat.LoadTable(ctx, catalog.ToIdentifier(ns, "events"))
	if err != nil {
		t.Fatal(err)
	}
	snaps := got.Metadata().Snapshots()
	if want := 2 + writers*each; len(snaps) != want {
		t.Errorf("%d snapshots, want %d: one per batch that landed", len(snaps), want)
	}
	if v := got.CurrentSnapshot().Summary.Properties["total-records"]; v != fmt.Sprint(total) {
		t.Errorf("total-records %q, want %d", v, total)
	}
	if got.Metadata().Version() != 2 || got.Properties()["write.parquet.compression-codec"] != "zstd" {
		t.Errorf("the table is v%d with codec %q", got.Metadata().Version(), got.Properties()["write.parquet.compression-codec"])
	}
	return outcome{Metadata: got.MetadataLocation(), Rows: total, SumQuantity: sum}
}

// leaveForTheOracle writes what DuckDB has to find, when CI asks for it.
func leaveForTheOracle(t *testing.T, o outcome) {
	t.Helper()
	path := os.Getenv("BREVIS_ICEBERG_TEST_ORACLE")
	if path == "" {
		return
	}
	b, _ := json.Marshal(o)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("writing %s for the oracle: %v", path, err)
	}
	t.Logf("oracle: %s", b)
}

// THE SQL CATALOG ON POSTGRES WITH S3: the configuration the stoa runs, and
// the one the DuckDB oracle reads afterwards.
//
// In the SQL catalog the Name is also the catalog_name the tables are
// registered under in Postgres, so every writer has to carry the same one;
// and one process opens one connection per Name, so the extra writers get
// catalogs of their own through Conn -- the same name, a connection each,
// which is what a second process would have.
func TestIntegrationSQLCatalogOnPostgresWithS3(t *testing.T) {
	dsn := envOrSkip(t, "BREVIS_ICEBERG_TEST_PG_DSN")
	store := storageOf(t)
	name := unique(t)
	out := exercise(t, func(writer int) Catalog {
		if writer == 0 {
			return Catalog{Name: name, Type: SQL, DSN: dsn, Warehouse: store.warehouse(), IO: store.io()}
		}
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		props := ice.Properties{"warehouse": store.warehouse()}
		maps.Copy(props, store.io())
		cat, err := sqlcat.NewCatalog(name, db, sqlcat.Postgres, props)
		if err != nil {
			t.Fatalf("writer %d: %v", writer, err)
		}
		return Catalog{Name: name, Conn: cat}
	})
	leaveForTheOracle(t, out)
}

// THE REST CATALOG, through the fixture, backed by the same S3: what a team
// on Polaris, Lakekeeper or Nessie gets.
func TestIntegrationRESTCatalogWithS3(t *testing.T) {
	uri := envOrSkip(t, "BREVIS_ICEBERG_TEST_REST_URI")
	store := storageOf(t)
	name := unique(t)
	exercise(t, func(writer int) Catalog {
		// A REST client's name is its own; a different one per writer is a
		// different client and a different connection to the same server.
		return Catalog{Name: fmt.Sprintf("%s-%d", name, writer), Type: REST, URI: uri, Warehouse: store.warehouse(), IO: store.io()}
	})
}
