package components

import (
	"testing"
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

func day(t time.Time, total, ok, failed, open int) postgres.Day {
	return postgres.Day{Date: t, Total: total, Succeeded: ok, Failed: failed, Open: open}
}

// thursday is a fixed today, 2026-10-29, a Thursday.
var thursday = time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)

// THE SUMMARY ANSWERS THE QUESTIONS BEFORE THE GRID: how many days it ran,
// how many failed, how long since the last failure, when that was, and since
// when it runs.
func TestTheCalendarSummarisesTheYear(t *testing.T) {
	d := func(m time.Month, n int) time.Time { return time.Date(2026, m, n, 0, 0, 0, 0, time.UTC) }
	s := SummariseCalendar([]postgres.Day{
		day(d(9, 1), 2, 2, 0, 0), day(d(10, 9), 2, 1, 1, 0), day(d(10, 28), 1, 1, 0, 0),
	}, thursday)
	if s.RanDays != 3 || s.FailedDays != 1 || s.Streak != 20 {
		t.Errorf("summary = %+v, want 3 days, 1 failed, 20 in a row", s)
	}
	if s.LastFailure == nil || !s.LastFailure.Equal(d(10, 9)) || s.Since == nil || !s.Since.Equal(d(9, 1)) {
		t.Errorf("last failure %v, since %v", s.LastFailure, s.Since)
	}

	clean := SummariseCalendar([]postgres.Day{day(d(10, 20), 1, 1, 0, 0)}, thursday)
	if clean.LastFailure != nil || clean.Streak != 10 {
		t.Errorf("a workflow that never failed: %+v, want 10 days since it began", clean)
	}
}
