// A REAL fetcher, with the real SDK, for the end-to-end test.
//
// It exists because everything else on the stages path was tested with a
// fake executor: the `@brevis:` line had never crossed an operating-system pipe,
// a bufio.Scanner and the runner's event loop. This binary makes it cross.
package main

import (
	"brevis.sh/brevis/sdk"
	"brevis.sh/brevis/sdk/from"
	"brevis.sh/brevis/sdk/to"
)

func main() {
	sdk.Run(sdk.Pipeline{
		Name:   "test-fetcher",
		Source: sdk.Source{From: from.Files{Path: "input.ndjson", Format: sdk.FormatNDJSON}},
		Transform: []sdk.Transformer{
			sdk.Compute("provider", func(map[string]any) (any, error) { return "test", nil }),
			sdk.Compute("entity", func(map[string]any) (any, error) { return "rows", nil }),
			sdk.ComputeText("source_key", sdk.Key("id")),
			sdk.ComputeText("record_ts", sdk.Field("ts")),
			sdk.IngestionID(),
		},
		Target: sdk.Target{To: to.Files{Path: "output/"}},
	})
}
