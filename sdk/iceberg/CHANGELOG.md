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

### Added: the module

- `sdk/iceberg` exists, with its own `go.mod` and no code yet beyond the
  package's documentation. The boundary comes first because it is the
  decision: what this module will weigh must never reach `sdk/go.mod`.
- CI builds, vets, tests and lints it on every push that touches `sdk/`,
  with `CGO_ENABLED=0`, the way every image in this repository is built.
