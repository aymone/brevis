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
