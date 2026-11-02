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
