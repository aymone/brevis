// One module, one binary, several subcommands -- which is what a real team
// does: one image, many entrypoints.
//
// It pins a PUBLISHED SDK rather than pointing at the tree, so this example is
// what somebody gets from `go get`. The repository's other examples do the
// opposite on purpose: they are a gate on the working tree.
//
// AND THE PIN HAS TO MOVE. It sat at v0.56.0, which is before the SDK said
// which destination a load wrote -- `landed` first shipped in v0.81.0 -- so
// this example ran correctly, wrote its files, and put NOTHING on /data.
// Anybody following it saw an empty catalog and no way to tell that from a
// broken one.
module brevis.sh/brevis/examples/full-pipeline/pipeline

go 1.26.0

require brevis.sh/brevis/sdk v0.84.1

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.12.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// TEMPORARY (the move to brevis.sh, 2026-11-02): no published SDK declares
// brevis.sh/brevis/sdk yet -- every version up to v0.87.0 declares the old
// github.com/AreteAcademy path -- so this builds against the SDK in the
// repository until sdk/v0.88.0 is tagged. The next change requires that
// version and drops this line. Do not tag this module while it is here.
replace brevis.sh/brevis/sdk => ../../../sdk
