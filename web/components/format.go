package components

import (
	"context"
	"time"

	"brevis.sh/brevis/web/i18n"
)

// ---------------------------------------------------------------------------
// Formatadores compartilhados
// ---------------------------------------------------------------------------

// Duration picks the unit by order of magnitude. "0.4s" says more than "400ms" in
// a column whose neighbour takes minutes.
//
// ENGLISH, for the screens that call it without a context; i18n.Elapsed is
// the same in the page's language, and a screen moves to it when it is
// translated.
func Duration(d *time.Duration) string {
	if d == nil {
		return "—"
	}
	return i18n.Elapsed(context.Background(), *d)
}

// UTCStamp is the full date in UTC, for values that are UTC by contract.
//
// The slot and the auto params are stored, injected and read in UTC -- a step
// gets `2026-03-11T01:00:00Z` in $BREVIS_AUTO_ADJUSTED_AT. Rendering those in
// the server's local time made the screen show `2026-03-10 22:00:00` beside a
// `date` of `2026-03-11`: one day apart, in the same grid, with nothing saying
// why, and a third answer for anyone who checked the environment.
//
// The screen agreeing with the contract is worth more than the screen agreeing
// with the clock of whoever ran the deployment.
func UTCStamp(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}
