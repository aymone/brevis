// A module of its own, for the reason sdk/metrics/otelmeter is one.
//
// The Iceberg destination is built on github.com/apache/iceberg-go, which
// brings Apache Arrow, the Parquet writer and the gocloud blob backends: 172
// packages and 40 MB on a binary, measured 2026-11-01 against the gateway.
// `to.Files` refused Parquet to keep exactly that weight away from a fetcher
// that writes a CSV, and putting iceberg-go in sdk/go.mod would hand it to
// every consumer anyway, because a module-level requirement is not pruned by
// what a consumer imports.
//
// So this go.mod requires the PUBLISHED sdk, the way otelmeter does, and no
// replace directive: a consumer compiles against what `require` says, and so
// does this module. It only breaks on a bump, which is the moment to find out.
//
// No cgo. iceberg-go is pure Go, and CGO_ENABLED=0 is what the Dockerfile and
// every gate in this repository build with.
module github.com/AreteAcademy/brevis/sdk/iceberg

go 1.26.0
