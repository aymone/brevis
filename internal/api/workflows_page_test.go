package api_test

import (
	"context"

	wf "brevis.sh/brevis/internal/domain/workflow"
)

// mapDefs is a published catalog that counts how it is read: one definition at
// a time, or all of them at once.
type mapDefs struct {
	defs       map[string]wf.Workflow
	one, every *int
}

func (d mapDefs) Definition(_ context.Context, slug string) (wf.Workflow, error) {
	*d.one++
	return d.defs[slug], nil
}

func (d mapDefs) Definitions(context.Context) (map[string]wf.Workflow, error) {
	*d.every++
	return d.defs, nil
}
