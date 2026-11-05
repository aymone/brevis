package schedule

import (
	"testing"
	"time"
)

// A REFUSED SLOT KEEPS ITS CODE AND DATA, and the same English in Error().
func TestSlotErrorSentences(t *testing.T) {
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		e    SlotError
		want string
	}{
		{SlotError{Code: SlotInFuture, Slot: at}, "slot 2026-10-26 10:00 UTC is in the future"},
		{SlotError{Code: SlotNoSchedule, Workflow: "w"}, `workflow "w" has no schedule: a slot is one of its schedule's runs`},
		{SlotError{Code: SlotNotOnCron, Slot: at, Cron: "0 9 * * *"}, `2026-10-26 10:00 UTC is not a slot of "0 9 * * *"`},
		{SlotError{Code: SlotNotOnCron, Slot: at, Cron: "0 6 * * *", Nearest: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), Zone: "America/Sao_Paulo"},
			`2026-10-26 10:00 UTC is not a slot of "0 6 * * *"; the nearest before it is 2026-10-26 09:00 UTC (2026-10-26 06:00 America/Sao_Paulo)`},
	} {
		if got := c.e.Error(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
