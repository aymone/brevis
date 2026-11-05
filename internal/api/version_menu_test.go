package api_test

import (
	"log/slog"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/updates"
)

type releaseFake struct {
	r     updates.Release
	newer bool
}

func (f releaseFake) Latest() (updates.Release, bool, bool) { return f.r, f.newer, true }

func versionUI(src api.UpdateSource) *api.UI {
	return api.NewUI(nil, nil, nil, nil, nil, catalogFake{}, branding.Default(), slog.New(slog.DiscardHandler)).
		WithEndpoints(endpointsFake{}).WithVersion("0.40.0", src)
}
