// Command gateway is the published gateway: no hooks.
//
// Which is the honest artifact for a compiled-hook design, and the limit is
// worth stating where somebody meets it. A hook is Go, compiled in, so an image
// built here can only carry hooks that were compiled here -- and none were.
// This image serves streams that declare no `hook:`, which is every stream that
// lands what it was sent.
//
// A hook of your own means a binary of your own, and it is ten lines:
//
//	func main() {
//	    hooks := gateway.NewHooks()
//	    hooks.MustRegister("enrich", enrich)
//	    gateway.Main(hooks, gateway.WithSinks(sinks))
//	}
//
// That is the same arrangement the SDK asks for -- the consumer compiles their
// own binary with their own code -- and it is why both products have one idiom
// rather than two. `gateway/example` is a working one.
//
// This binary registers ALL SIX sinks and both object stores, which is what
// makes it 49 MB. Anyone building their own registers what they use and pays
// for that alone: postgres and a local dead letter is 10 MB. See
// cmd/gateway-slim, which is the same idea shipped as an image.
package main

import (
	"brevis.sh/brevis/gateway"
	"brevis.sh/brevis/gateway/metastore/memcached"
	"brevis.sh/brevis/gateway/metastore/redis"
	"brevis.sh/brevis/gateway/sink/autotable"
	"brevis.sh/brevis/gateway/sink/bigquery"
	"brevis.sh/brevis/gateway/sink/files"
	"brevis.sh/brevis/gateway/sink/mysql"
	"brevis.sh/brevis/gateway/sink/postgres"
	"brevis.sh/brevis/gateway/sink/pubsub"
	"brevis.sh/brevis/gateway/sink/redshift"
	"brevis.sh/brevis/gateway/store/gcs"
	"brevis.sh/brevis/gateway/store/s3"
)

func main() {
	sinks := gateway.NewSinks()
	sinks.MustRegister(pubsub.Sink, pubsub.New)
	sinks.MustRegister(postgres.Sink, postgres.New)
	sinks.MustRegister(mysql.Sink, mysql.New)
	sinks.MustRegister(bigquery.Sink, bigquery.New)
	sinks.MustRegister(redshift.Sink, redshift.New)
	sinks.MustRegister(files.Sink, files.New)
	sinks.MustRegister(autotable.Sink, autotable.New)

	// What describe uses to name each destination without opening it.
	sinks.MustRegisterLocator(pubsub.Sink, pubsub.Locate)
	sinks.MustRegisterLocator(postgres.Sink, postgres.Locate)
	sinks.MustRegisterLocator(mysql.Sink, mysql.Locate)
	sinks.MustRegisterLocator(bigquery.Sink, bigquery.Locate)
	sinks.MustRegisterLocator(redshift.Sink, redshift.Locate)
	sinks.MustRegisterLocator(files.Sink, files.Locate)
	sinks.MustRegisterLocator(autotable.Sink, autotable.Locate)

	metastores := gateway.NewMetastores()
	metastores.MustRegister(redis.Name, redis.Open)
	metastores.MustRegister(memcached.Name, memcached.Open)

	stores := gateway.NewStores()
	stores.MustRegister(s3.Scheme, s3.Open)
	stores.MustRegister(gcs.Scheme, gcs.Open)

	gateway.Main(nil, gateway.WithSinks(sinks), gateway.WithStores(stores),
		gateway.WithMetastores(metastores))
}
