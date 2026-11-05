package components

import (
	"time"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// CalendarSummary is what the calendar says before its grid: how many days
// it ran, how many failed, how many days since the last failure -- or since
// it began, if it never failed -- when that was, and since when it runs.
type CalendarSummary struct {
	RanDays, FailedDays, Streak int
	LastFailure, Since          *time.Time
}

// SummariseCalendar counts the days that ran and failed, and how long since the last
// failure -- or since the first run, when there was none. The squares, months and
// weekday names are the app's: it draws the calendar and says its words.
func SummariseCalendar(days []postgres.Day, now time.Time) CalendarSummary {
	today := now.UTC().Truncate(24 * time.Hour)
	var s CalendarSummary
	for _, d := range days {
		if d.Total == 0 {
			continue
		}
		day := d.Date.UTC().Truncate(24 * time.Hour)
		s.RanDays++
		if s.Since == nil || day.Before(*s.Since) {
			s.Since = &day
		}
		if d.Failed > 0 {
			s.FailedDays++
			if s.LastFailure == nil || day.After(*s.LastFailure) {
				s.LastFailure = &day
			}
		}
	}
	switch {
	case s.LastFailure != nil:
		s.Streak = int(today.Sub(*s.LastFailure).Hours() / 24)
	case s.Since != nil:
		s.Streak = int(today.Sub(*s.Since).Hours()/24) + 1
	}
	return s
}
