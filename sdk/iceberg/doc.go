// Package iceberg will be the SDK's Apache Iceberg destination: a table a
// pipeline appends to with sdk.Load, through a REST or SQL catalog, on S3,
// GCS or local disk, written as Parquet by github.com/apache/iceberg-go.
//
// This commit is the module and nothing else. The module exists first because
// its boundary is the decision: iceberg-go brings Arrow, and Arrow is the
// weight to.Files refused on purpose. A separate go.mod keeps that weight out
// of every consumer who does not write this import -- out of their binary and
// out of their go.sum alike (see the comment in go.mod).
//
// What the destination will and will not do is written down before the code,
// in the spec this module is built from, and the short version is:
//
//   - append only. Dedup and schema evolution are refused by name, never
//     accepted and ignored; a retried batch lands once through the BatchKey
//     the SDK already carries, recorded in the snapshot's summary.
//   - no type inference. CreateTable needs a Schema, as every SQL destination
//     in this SDK does, and the nine SDK column types map to Iceberg types by
//     a fixed table.
//   - the object-store backend is the consumer's import, not this package's:
//     a lake on S3 blank-imports "github.com/apache/iceberg-go/io/gocloud/s3",
//     so a table on local disk compiles none of the AWS SDK.
//   - the table is loaded from the catalog before every write, and a commit
//     that conflicts with another writer is retried here, with backoff and
//     jitter, because the catalog's own retry is opt-in and does not cover
//     every conflict a SQL catalog raises.
package iceberg
