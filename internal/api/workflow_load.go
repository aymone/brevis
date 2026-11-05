package api

import (
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/components"
)

// loadView is the "What it loads" panel's data: the four series the server
// page draws, already computed (components.Trend) so the SPA cannot disagree
// with it about a number or a delta. Absent (null) when the workflow has no
// load history, like the page, which then draws no panel.
// loadView is the "What it loads" panel as data: the app names the series, writes the numbers in their
// unit and says what the delta means.
type loadView struct {
	// From and To are the first and last day of the window, YYYY-MM-DD.
	From   string           `json:"from"`
	To     string           `json:"to"`
	Series []loadSeriesView `json:"series"`
}

type loadSeriesView struct {
	// Key is "rows", "bytes", "extract" or "load"; Unit is "count", "size" (bytes) or "seconds".
	Key    string  `json:"key"`
	Unit   string  `json:"unit"`
	Latest float64 `json:"latest"`
	// Delta is "", "new", "steady" or "pct" (Pct is the change in percent, rounded).
	Delta  string          `json:"delta"`
	Pct    int             `json:"pct"`
	Hue    string          `json:"hue"` // a text-* token class from the compiled sheet
	Path   string          `json:"path"`
	Area   string          `json:"area"`
	Flat   bool            `json:"flat"`
	Points []loadPointView `json:"points"`
}

type loadPointView struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Date  string  `json:"date"` // YYYY-MM-DD
	Value float64 `json:"value"`
}

func loadOf(days []postgres.LoadDay) *loadView {
	if len(days) == 0 {
		return nil
	}
	out := &loadView{Series: []loadSeriesView{}}
	out.From, out.To = components.TrendWindow(days)
	for _, s := range components.Trend(days) {
		sv := loadSeriesView{Key: s.Key, Unit: s.Unit, Latest: s.Latest, Delta: s.Delta, Pct: s.Pct, Hue: s.Hue,
			Path: s.Path, Area: s.Area, Flat: s.Flat, Points: []loadPointView{}}
		for _, p := range s.Points {
			sv.Points = append(sv.Points, loadPointView{X: p.X, Y: p.Y, Date: p.Date, Value: p.Value})
		}
		out.Series = append(out.Series, sv)
	}
	return out
}
