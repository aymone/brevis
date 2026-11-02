// A module of its own, and that is the whole point.
//
// sdk/metrics/otelmeter carries the OTLP exporter, which carries protobuf and
// gRPC. Putting it in sdk/go.mod made `go mod tidy` resolve the WHOLE graph
// upward -- BigQuery 1.50 to 1.72, storage 1.30 to 1.56 -- and a consumer of
// sdk/to/bigquery went from 460 packages to 736 without importing anything new.
// The module graph moved underneath them.
//
// Package-level pruning does not protect a consumer from a MODULE-level version
// bump. A separate go.mod is what makes "you only pay for what you import" true
// at the module boundary too.
//
// It requires a PUBLISHED SDK version and carries no `replace`: a replace is
// ignored by consumers, so a module that needs one to build is a module that
// does not build for anybody else (see 3142c16).
module brevis.sh/brevis/sdk/metrics/otelmeter

go 1.26.0

require (
	brevis.sh/brevis/sdk v0.86.1
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.47.0
	go.opentelemetry.io/otel/metric v1.47.0
	go.opentelemetry.io/otel/sdk/metric v1.47.0
	go.opentelemetry.io/proto/otlp v1.11.1
	google.golang.org/protobuf v1.36.12
)

require (
	cloud.google.com/go/auth/oauth2adapt v0.3.1 // indirect
	cloud.google.com/go/bigquery v1.86.0 // indirect
	cloud.google.com/go/iam v1.14.0 // indirect
	cloud.google.com/go/storage v1.70.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/gax-go/v2 v2.27.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20260924152758-ed294f943157 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/api v0.307.0 // indirect
	google.golang.org/genproto v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
	google.golang.org/grpc v1.84.0 // indirect
)

// TEMPORARY (the move to brevis.sh, 2026-11-02): no published SDK declares
// brevis.sh/brevis/sdk yet -- every version up to v0.87.0 declares the old
// github.com/AreteAcademy path -- so this builds against the SDK in the
// repository until sdk/v0.88.0 is tagged. The next change requires that
// version and drops this line. Do not tag this module while it is here.
replace brevis.sh/brevis/sdk => ../../../sdk
