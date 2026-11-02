# Changelog — the Iceberg destination

`github.com/AreteAcademy/brevis/sdk/iceberg`, published from an
`sdk/iceberg/v*` tag.

Its own version, on purpose. The destination is a separate Go module because
iceberg-go brings Arrow, and a separate module is what keeps that weight out
of every SDK consumer who does not import it. A module that versions apart
from the SDK also releases apart from it: a fix here must not force an SDK
release, and an SDK release must not claim a maturity this module does not
have. The SDK's versions are in [`CHANGELOG.md`](../../CHANGELOG.md).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [SemVer](https://semver.org/).

---

## [Unreleased]

### Added: the commit, with the batch key and a retry

- Every snapshot this destination writes carries `brevis.batch-key`, the
  `WriteOptions.BatchKey`, and `brevis.sdk`, the writer, in its summary. A
  `Write` whose key already names a snapshot writes nothing and reports
  `Strategy: "already-committed"` with the rows that snapshot added and no
  `Objects`: a replay lands once, which is what the gateway's dead letter
  needs since `Dedup` is refused. The key is checked on every attempt, so
  a commit that landed and then failed to report is recognised by the
  retry instead of written twice.
- A conflicting commit is retried: the table is reloaded and the append
  redone, with backoff from 50 ms doubling to 2 s and jitter, up to
  `Table.CommitRetries` attempts (ten by default) within
  `Table.CommitWithin` (thirty seconds by default), both listed by
  `Configuration()` when set. Both forms the spike S3 measured count as a
  conflict -- iceberg-go's `ErrCommitFailed` and the raw
  `requirement failed` the catalogs raise before the compare-and-swap --
  and nothing else does: a refusal from the catalog is reported as it
  came, once. Two writers on one table in goroutines land every batch,
  under the race detector.

### Added: `iceberg.Table`, the destination

- `Table{Catalog, Namespace, Name, PartitionBy, CreateTable}` satisfies the
  SDK's `Writer`, `Locator` and `Configured`. `Write` refuses first, loads
  the table on every call, creates it when asked and missing, reconciles
  the batch against it, converts it in batches and appends them as one
  snapshot. The result carries the rows and the Parquet bytes from the
  snapshot's summary, the data files it added, `Strategy: "append"`,
  `Format: "parquet"` and whether the table is new. A `Write` with no rows
  creates the table if asked and appends nothing.
- A created table is format v2, `write.parquet.compression-codec = zstd`,
  `commit.retry.num-retries = 4`, partitioned by `day(PartitionBy)` when
  there is one, in a namespace created if missing. `CreateTable` is
  tri-state like BigQuery's: explicit first, then the engine's first run or
  its `create_table` parameter. Without `Schema`, creation is refused
  listing the batch's columns. On an existing table, `PartitionBy` has to
  be the table's own `day(column)` or it is refused saying what the table
  has; no `PartitionBy` makes no claim.
- Refusals, before a catalog opens: `Dedup` set to anything but none, a
  `Default` or a `Unique` on a declared column -- an Iceberg table has
  nothing that applies either -- and a `PartitionBy` set both on the Table
  and on the Target to different columns. `Evolve` is not an option here:
  the destination has no such field, and a divergence is `Reconcile`'s
  error, never an ALTER.
- `Locate()` is `iceberg://{catalog}/{namespace}/{table}`, a dotted
  namespace one segment, without opening anything. `Describe()` is
  `catalog.namespace.table`. `Configuration()` lists the catalog, its type,
  the server without userinfo, the warehouse, the table and the options,
  and never a DSN, a token, a credential or an IO key.
- The matrix row lives in the module's `capabilities_test.go`, since the
  SDK's cannot import a module that depends on it: Dedup refused naming
  `Dedup` and `BatchKey`, `CreateTable` from `Schema`, no `Evolve` field.
- Tested end to end without a service, through iceberg-go's filesystem
  catalog on a temporary directory handed over as `Conn`.

### Added: the catalog, REST and SQL on Postgres, opened once per process

- `Catalog{Name, Type, URI, Credential, Token, Warehouse, DSN, IO, Conn}`
  and `Open(ctx)`. `REST` goes through iceberg-go's client, which does the
  `/v1/config` handshake with the warehouse and carries the token or
  exchanges the credential; `SQL` goes through `database/sql` on pgx with
  the `postgres` dialect, and the catalog's own tables are created on first
  use. One catalog per `Name` per process, the way the SDK's Google clients
  are one per process: eight goroutines opening one name produce one
  handshake. The same `Name` with another configuration is refused naming
  the fields that differ and never their values. A failed open is not
  cached. `Conn` hands over an opened catalog and `Open` returns it as it is.
- The IO properties reach the catalog verbatim -- they are iceberg-go's keys,
  `s3.endpoint`, `s3.access-key-id`, `s3.region`, so there is no translation
  to lag behind -- with the warehouse alongside and nothing else: a DSN, a
  token or a credential is not a file IO property. A refusal names the
  field: a `Type` this destination does not open, a REST catalog without
  `URI`, a SQL catalog without `DSN`, a catalog without `Name`.
- The module now requires `github.com/jackc/pgx/v5`, at the version the
  published sdk already pins. The REST client brings `aws-sdk-go-v2` into
  the graph for its SigV4 option; the pruning case in CI will say what it
  weighs on a binary.

### Added: the asymmetric reconciliation, as a pure function

- `Reconcile` takes the table's schema, the batch and the write options and
  gives back the schema `Records` converts with: every table column in the
  table's order, typed as the consumer declared it when they did and as the
  table holds it when they did not, required where the table or the
  declaration says so. The rule is the SDK's 2.8: a field the table does not
  have is refused naming it, a table column the records do not carry stays
  null, incompatible types are refused naming the column and both types --
  the declaration against the table, and the table against what this
  destination writes -- and a required column no record carries is refused
  before a Parquet file with an all-null required column exists. A `Columns`
  entry the Transform chain did not produce is its own refusal, distinct
  from a table column nobody sent. No catalog, no context.

### Added: envelopes become Arrow batches, 65,536 rows at a time

- `Records` turns a `Write`'s envelopes into Arrow record batches in the
  declared schema's shape, `BatchRows` rows per batch, with one batch alive
  at a time: a million rows of ten columns go through with the heap under
  512 MB, and the test measures it. Every value is converted by the column's
  declared type and nothing else: `int64` takes an integer or an integral
  float, `numeric` takes plain decimal text with at most nine fractional
  digits or a number, `timestamp` takes RFC 3339 or a `time.Time`, `date`
  takes `YYYY-MM-DD` or a `time.Time`'s calendar day, `json` takes a document
  serialised or already serialised, `bytes` takes `[]byte` or standard
  base64. A value that does not fit is refused naming the record, the column,
  the declared type and what arrived; a number into a timestamp is refused
  rather than guessed between seconds and milliseconds. An absent or null
  value is null in a column that takes it, and a refusal in a required one.

### Added: the type map, as two pure functions

- `IcebergSchema` and `ArrowSchema` turn a declared `sdk.Schema` into the
  table's schema and the rows' in-memory shape: the same columns in the same
  order, field ids 1..n in that order, required where declared. One SDK type
  becomes one Iceberg type and one Arrow type, by a written table: `numeric`
  is `decimal(38, 9)`, `timestamp` is `timestamptz` in microseconds, `json`
  is the serialised document as a `string`. A type outside the table is
  refused naming the column and the type; a schema with no columns is
  refused before it becomes a table with none.
- The module now requires `github.com/apache/iceberg-go v0.7.0`,
  `github.com/apache/arrow-go/v18 v18.8.0` and the published
  `github.com/AreteAcademy/brevis/sdk v0.87.0`.

### Added: the module

- `sdk/iceberg` exists, with its own `go.mod` and no code yet beyond the
  package's documentation. The boundary comes first because it is the
  decision: what this module will weigh must never reach `sdk/go.mod`.
- CI builds, vets, tests and lints it on every push that touches `sdk/`,
  with `CGO_ENABLED=0`, the way every image in this repository is built.
