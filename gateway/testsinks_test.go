package gateway_test

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

// everything is what cmd/gateway registers, so the tests exercise the
// published image's build rather than a set of their own.
//
// A set of their own is the trap: the tests would pass on sinks the image does
// not carry, and the first person to find out would be whoever deployed it.
func everything() []gateway.Option {
	sinks := gateway.NewSinks()
	sinks.MustRegister(pubsub.Sink, pubsub.New)
	sinks.MustRegister(postgres.Sink, postgres.New)
	sinks.MustRegister(mysql.Sink, mysql.New)
	sinks.MustRegister(bigquery.Sink, bigquery.New)
	sinks.MustRegister(redshift.Sink, redshift.New)
	sinks.MustRegister(files.Sink, files.New)
	sinks.MustRegister(autotable.Sink, autotable.New)

	metastores := gateway.NewMetastores()
	metastores.MustRegister(redis.Name, redis.Open)
	metastores.MustRegister(memcached.Name, memcached.Open)

	stores := gateway.NewStores()
	stores.MustRegister(s3.Scheme, s3.Open)
	stores.MustRegister(gcs.Scheme, gcs.Open)

	return []gateway.Option{gateway.WithSinks(sinks), gateway.WithStores(stores),
		gateway.WithMetastores(metastores)}
}

// with is everything() plus the options a test adds.
func with(opts ...gateway.Option) []gateway.Option {
	return append(everything(), opts...)
}
