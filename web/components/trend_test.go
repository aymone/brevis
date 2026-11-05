package components

import (
	"math"
	"testing"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func loadDay(n int, runs int, rows, bytesOut, extractMs, loadMs int64) postgres.LoadDay {
	return postgres.LoadDay{
		Date:      time.Date(2026, 3, n, 0, 0, 0, 0, time.UTC),
		Runs:      runs,
		Rows:      rows,
		BytesOut:  bytesOut,
		ExtractMs: extractMs,
		LoadMs:    loadMs,
	}
}

// A day with four runs moved four times as much, and that is a schedule getting
// denser -- not a pipeline getting bigger.
//
// This is the mistake the panel exists to avoid making: somebody switches a
// workflow from daily to every six hours, the "rows" line quadruples overnight,
// and the next person spends an afternoon looking for a source that did not
// change.
func TestVolumeIsPerRunAndNotPerDay(t *testing.T) {
	series := Trend([]postgres.LoadDay{
		loadDay(1, 1, 1000, 0, 0, 0),
		loadDay(2, 4, 4000, 0, 0, 0), // four runs, same size each
	})
	rows := series[0]
	if rows.Key != "rows" || rows.Unit != "count" {
		t.Fatalf("the first series is %q (%s)", rows.Key, rows.Unit)
	}
	if rows.Latest != 1000 {
		t.Errorf("latest = %v; four runs of 1,000 is still 1,000 a run", rows.Latest)
	}
	if rows.Delta != "steady" {
		t.Errorf("delta = %q; nothing about the pipeline changed", rows.Delta)
	}
}

// The delta compares the last day against the FIRST.
//
// Day over day on a daily pipeline is one sample against one sample: a slow
// morning at the vendor reads as a 40% regression and means nothing. The
// question is about the window.
func TestTheDeltaIsAcrossTheWindowAndNotAgainstYesterday(t *testing.T) {
	// Climbs steadily, then dips on the final day. Against yesterday that dip
	// is the headline; across the window the climb is.
	series := Trend([]postgres.LoadDay{
		loadDay(1, 1, 0, 0, 0, 10000),
		loadDay(2, 1, 0, 0, 0, 14000),
		loadDay(3, 1, 0, 0, 0, 18000),
		loadDay(4, 1, 0, 0, 0, 17000),
	})
	load := series[3]
	if load.Key != "load" {
		t.Fatalf("the fourth series is %q", load.Key)
	}
	if load.Delta != "pct" || load.Pct != 70 {
		t.Errorf("delta = %q %d, wanted +70%% (10s to 17s across the window)", load.Delta, load.Pct)
	}
	// And a load taking longer is unambiguously worse, so it is painted so.
	if load.Hue != "text-state-failed" {
		t.Errorf("a load 70%% slower is drawn %q", load.Hue)
	}
}

// More rows is NOT worse, and the screen must not say it is.
//
// A pipeline whose volume doubled may be a healthy business or may be
// double-reading its source. Painting that red would be the screen guessing at
// something only the reader knows.
func TestGrowingVolumeIsNotPaintedAsAFailure(t *testing.T) {
	series := Trend([]postgres.LoadDay{
		loadDay(1, 1, 1000, 0, 0, 0),
		loadDay(2, 1, 5000, 0, 0, 0),
	})
	rows := series[0]
	if rows.Delta != "pct" || rows.Pct != 400 {
		t.Fatalf("delta = %q %d", rows.Delta, rows.Pct)
	}
	if rows.Hue == "text-state-failed" {
		t.Error("a pipeline loading more rows was painted as a failure")
	}
}

// Bytes per row is the note's own question: a load that doubles in size while
// its row count holds flat is a schema that grew a column.
func TestBytesPerRowCatchesASchemaThatGrew(t *testing.T) {
	series := Trend([]postgres.LoadDay{
		loadDay(1, 1, 1000, 100_000, 0, 0), // 100 B a row
		loadDay(2, 1, 1000, 200_000, 0, 0), // 200 B a row, same rows
	})
	bpr := series[1]
	if bpr.Key != "bytes" || bpr.Unit != "size" {
		t.Fatalf("the second series is %q (%s)", bpr.Key, bpr.Unit)
	}
	if bpr.Latest != 200 {
		t.Errorf("latest = %v", bpr.Latest)
	}
	if bpr.Delta != "pct" || bpr.Pct != 100 {
		t.Errorf("delta = %q %d; the rows did not change and the bytes doubled", bpr.Delta, bpr.Pct)
	}
}

// One day is not a trend, and the panel says so rather than drawing a flat line
// that reads as "stable".
func TestOneDayDrawsNoLine(t *testing.T) {
	for _, s := range Trend([]postgres.LoadDay{loadDay(1, 1, 1000, 100, 5000, 5000)}) {
		if !s.Flat {
			t.Errorf("%s drew a trend from a single day", s.Key)
		}
		if s.Path != "" {
			t.Errorf("%s drew a line from a single day: %q", s.Key, s.Path)
		}
		if len(s.Points) != 0 {
			t.Errorf("%s has points for a single day", s.Key)
		}
	}
}

// Nothing to divide by. A day whose rows are zero has no bytes-per-row, and an
// infinity on a chart is worse than a gap.
func TestNothingIsDividedByZero(t *testing.T) {
	series := Trend([]postgres.LoadDay{
		loadDay(1, 0, 0, 0, 0, 0),
		loadDay(2, 0, 0, 500, 0, 0),
	})
	for _, s := range series {
		if math.IsInf(s.Latest, 0) || math.IsNaN(s.Latest) {
			t.Errorf("%s = %v", s.Key, s.Latest)
		}
		for _, p := range s.Points {
			if math.IsInf(p.Value, 0) || math.IsNaN(p.Value) {
				t.Errorf("%s point: %v", s.Key, p.Value)
			}
		}
	}
}

// No history at all: every series reports empty rather than drawing zeros. The
// page checks this before rendering the panel, and this is what makes that
// check possible.
func TestNoHistoryIsEmptyAndNotZero(t *testing.T) {
	for _, s := range Trend(nil) {
		if !s.Empty {
			t.Errorf("%s claims to have data", s.Key)
		}
		if s.Path != "" {
			t.Errorf("%s drew a line out of nothing", s.Key)
		}
	}
	if from, to := TrendWindow(nil); from != "" || to != "" {
		t.Errorf("TrendWindow(nil) = %q, %q", from, to)
	}
}

// The caption names the range the reader is looking at.
func TestTheWindowIsNamed(t *testing.T) {
	from, to := TrendWindow([]postgres.LoadDay{loadDay(1, 1, 0, 0, 0, 0), loadDay(28, 1, 0, 0, 0, 0)})
	if from != "2026-03-01" || to != "2026-03-28" {
		t.Errorf("TrendWindow = %q, %q", from, to)
	}
}
