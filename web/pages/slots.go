package pages

import (
	"context"
	"time"

	sch "brevis.sh/brevis/internal/domain/schedule"
)

// Slot is one past slot of a schedule, and the window it processes: from the
// slot before it to itself, the same window its interval_start and
// interval_end are when it runs.
type Slot struct {
	At, From time.Time
}

// RecentSlots is a schedule's last n slots up to now, newest first, in UTC:
// what "Run another slot" offers. A cron only says what comes next, so it
// walks forward from a window behind now, widening the window until it holds
// n slots and the one before the oldest -- a quarter-hour finds them in the
// first, a monthly in the last. A schedule that cannot be read, or never
// fires, has none.
func RecentSlots(cron, zone string, now time.Time, n int) []Slot {
	if cron == "" || n <= 0 {
		return nil
	}
	c, loc, err := sch.Schedule{Cron: cron, Timezone: zone}.Parse()
	if err != nil {
		return nil
	}
	var fires []time.Time
	for back := 6 * time.Hour; back <= 8*366*24*time.Hour; back *= 4 {
		fires = fires[:0]
		for f := c.Next(now.Add(-back).In(loc)); !f.IsZero() && !f.After(now); f = c.Next(f) {
			fires = append(fires, f.UTC())
		}
		if len(fires) > n {
			break
		}
	}
	// The oldest fire found has no window unless one before it is known.
	out := make([]Slot, 0, n)
	for i := len(fires) - 1; i >= 1 && len(out) < n; i-- {
		out = append(out, Slot{At: fires[i], From: fires[i-1]})
	}
	return out
}

// SlotLabel is a slot as the Run drawer lists it, for the app's JSON.
func SlotLabel(ctx context.Context, s Slot, zone string) string { return slotLabel(ctx, s, zone) }
