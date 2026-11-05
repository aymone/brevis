package components

import (
	"fmt"
	"math"
	"strings"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// The sparkline's geometry, in the SVG's own units so one viewBox scales the
// whole drawing and no media query is needed. Same approach as the calendar.
const (
	trendW      = 300
	trendH      = 56
	trendPadTop = 4
	trendPadBot = 10
)

// TrendSeries is one line of the load trend, ready to draw. It holds numbers
// and codes, never words: the app names the series (Key), writes the numbers
// in the unit the series is measured in (Unit) and says what the delta means.
//
// Everything the drawing needs is computed here rather than in the markup: a
// component that does arithmetic is a file where a division by zero becomes
// a blank page during an incident.
type TrendSeries struct {
	// Key is "rows", "bytes", "extract" or "load"; Unit is "count", "size" (bytes) or "seconds".
	Key, Unit string
	Latest    float64 // the current value
	// Delta is how it moved across the window: "" (nothing to say), "new" (from nothing to something),
	// "steady" or "pct", with Pct the percentage, rounded.
	Delta  string
	Pct    int
	Hue    string // the CSS class for Delta
	Path   string // the polyline's points
	Area   string // the same, closed to the baseline
	Points []TrendPoint
	Empty  bool // no data at all: the panel says so instead of drawing a flat line
	Flat   bool // one day only, so there is no trend yet
}

// TrendPoint is one day on the line: where it is drawn, and what it was.
type TrendPoint struct {
	X, Y  int
	Date  string // YYYY-MM-DD
	Value float64
}

// Trend turns a workflow's load history into the four series the screen draws.
//
// Four, and these four, because they are the note's own questions: is the
// dataset growing, is each row getting fatter, is the source getting slower, is
// the destination getting slower. A fifth would be a number nobody asked for,
// and this panel has to be readable at a glance during an incident.
func Trend(days []postgres.LoadDay) []TrendSeries {
	rows := make([]float64, len(days))
	bpr := make([]float64, len(days))
	ext := make([]float64, len(days))
	load := make([]float64, len(days))
	for i, d := range days {
		// Per RUN, not per day. A day with four runs moved four times as much
		// and that is not the pipeline getting slower -- summing here would
		// make a denser schedule look like a regression.
		runs := float64(d.Runs)
		if runs == 0 {
			runs = 1
		}
		rows[i] = float64(d.Rows) / runs
		bpr[i] = float64(d.BytesPerRow())
		ext[i] = float64(d.ExtractMs) / 1000
		load[i] = float64(d.LoadMs) / 1000
	}

	return []TrendSeries{
		buildTrend("rows", "count", days, rows, false),
		buildTrend("bytes", "size", days, bpr, false),
		buildTrend("extract", "seconds", days, ext, true),
		buildTrend("load", "seconds", days, load, true),
	}
}

// buildTrend draws one series.
//
// `worseWhenUp` decides only the COLOUR of the trendDelta, and only where the
// direction has a meaning: a load taking longer is worse, and more rows is just
// more rows -- a pipeline whose volume doubled may be healthy or may be
// double-reading, and painting that red would be the screen guessing.
func buildTrend(key, unit string, days []postgres.LoadDay, values []float64, worseWhenUp bool) TrendSeries {
	s := TrendSeries{Key: key, Unit: unit}
	if len(values) == 0 {
		s.Empty = true
		return s
	}
	s.Latest = values[len(values)-1]
	if len(values) == 1 {
		s.Flat = true
		return s
	}

	lo, hi := values[0], values[0]
	for _, v := range values {
		lo = min(lo, v)
		hi = max(hi, v)
	}
	// A flat series still draws a line, through the middle. Scaling a constant
	// to the full height would turn rounding noise into a mountain range.
	span := hi - lo
	flat := span == 0

	usable := float64(trendH - trendPadTop - trendPadBot)
	var line, area strings.Builder
	for i, v := range values {
		x := 0
		if len(values) > 1 {
			x = i * trendW / (len(values) - 1)
		}
		y := trendPadTop + int(usable/2)
		if !flat {
			y = trendPadTop + int((1-(v-lo)/span)*usable)
		}
		fmt.Fprintf(&line, "%d,%d ", x, y)
		s.Points = append(s.Points, TrendPoint{X: x, Y: y, Date: days[i].Date.Format("2006-01-02"), Value: v})
	}
	s.Path = strings.TrimSpace(line.String())

	// The area is the same line closed along the bottom edge, which is what
	// gives the sparkline weight without a second data pass.
	fmt.Fprintf(&area, "0,%d ", trendH)
	area.WriteString(s.Path)
	fmt.Fprintf(&area, " %d,%d", trendW, trendH)
	s.Area = area.String()

	s.Delta, s.Pct, s.Hue = trendDelta(values, worseWhenUp)
	return s
}

// trendDelta compares the last day against the FIRST, not against the day before.
//
// Day-over-day on a daily pipeline is one sample against one sample, and it
// swings on anything: a slow morning at the vendor reads as a 40% regression
// and says nothing. The question this panel answers is "is it getting worse",
// which is a question about the window.
func trendDelta(values []float64, worseWhenUp bool) (kind string, pct int, hue string) {
	first, last := values[0], values[len(values)-1]
	if first == 0 {
		// From nothing to something is not a percentage. Saying "+Inf%" or
		// "+100%" would both be inventing a denominator.
		if last == 0 {
			return "", 0, "text-muted"
		}
		return "new", 0, "text-muted"
	}
	change := (last - first) / first * 100
	// Under five percent is noise on a series this short, and a badge that
	// moves every day is a badge nobody reads.
	if change > -5 && change < 5 {
		return "steady", 0, "text-muted"
	}
	hue = "text-muted"
	if worseWhenUp {
		hue = "text-state-success"
		if change > 0 {
			hue = "text-state-failed"
		}
	}
	return "pct", int(math.RoundToEven(change)), hue
}

// TrendWindow is the span the panel covers, for the caption: the first and the last day.
func TrendWindow(days []postgres.LoadDay) (from, to string) {
	if len(days) == 0 {
		return "", ""
	}
	return days[0].Date.Format("2006-01-02"), days[len(days)-1].Date.Format("2006-01-02")
}

// TrendDays is how far back the trend reads. Ninety rather than the calendar's
// year: a chart with 364 points on 300 units of width draws under one point per
// pixel, and "is it getting worse" is a question about the recent past.
const TrendDays = 90
