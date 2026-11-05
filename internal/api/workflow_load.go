package api

import (
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
)

// loadView is the "What it loads" panel's data: the four series the server
// page draws, already computed (components.Trend) so the SPA cannot disagree
// with it about a number or a delta. Absent (null) when the workflow has no
// load history, like the page, which then draws no panel.
type loadView struct {
	Window string           `json:"window"`
	Series []loadSeriesView `json:"series"`
}

type loadSeriesView struct {
	Label  string          `json:"label"`
	Latest string          `json:"latest"`
	Delta  string          `json:"delta"`
	Hue    string          `json:"hue"` // a text-* token class from the compiled sheet
	Note   string          `json:"note"`
	Path   string          `json:"path"`
	Area   string          `json:"area"`
	Flat   bool            `json:"flat"`
	Points []loadPointView `json:"points"`
}

type loadPointView struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Tooltip string `json:"tooltip"`
}

func loadOf(days []postgres.LoadDay) *loadView {
	if len(days) == 0 {
		return nil
	}
	out := &loadView{Window: components.TrendWindow(days), Series: []loadSeriesView{}}
	for _, s := range components.Trend(days) {
		sv := loadSeriesView{Label: s.Label, Latest: s.Latest, Delta: s.Delta, Hue: s.Hue, Note: s.Note,
			Path: s.Path, Area: s.Area, Flat: s.Flat, Points: []loadPointView{}}
		for _, p := range s.Points {
			sv.Points = append(sv.Points, loadPointView{X: p.X, Y: p.Y, Tooltip: p.Tooltip})
		}
		out.Series = append(out.Series, sv)
	}
	return out
}
