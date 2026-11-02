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

require (
	github.com/AreteAcademy/brevis/sdk v0.87.0
	github.com/apache/arrow-go/v18 v18.8.0
	github.com/apache/iceberg-go v0.7.0
)

require (
	github.com/geoarrow/geoarrow-go v0.0.0-20260403143023-f54751c3e3a1 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/twmb/avro v1.8.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20260603202125-055de637280b // indirect
	golang.org/x/sys v0.47.0 // indirect
)
