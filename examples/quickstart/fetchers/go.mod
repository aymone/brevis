// A module of its own, and against the PUBLISHED SDK -- which is what a team
// copying this example would write. Nothing here depends on the Brevis
// checkout.
module quickstart

go 1.26.0

require brevis.sh/brevis/sdk v0.51.0

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/net v0.62.0 // indirect
)

// TEMPORARY (the move to brevis.sh, 2026-11-02): no published SDK declares
// brevis.sh/brevis/sdk yet -- every version up to v0.87.0 declares the old
// github.com/AreteAcademy path -- so this builds against the SDK in the
// repository until sdk/v0.88.0 is tagged. The next change requires that
// version and drops this line. Do not tag this module while it is here.
replace brevis.sh/brevis/sdk => ../../../sdk
