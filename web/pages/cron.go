package pages

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"brevis.sh/brevis/web/i18n"
)

// CronTextIn is CronText in the context's language.
func CronTextIn(ctx context.Context, expr string) string {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return ""
	}
	min, hour, dom, month, dow := f[0], f[1], f[2], f[3], f[4]
	if month != "*" {
		return ""
	}

	if hour == "*" && dom == "*" && dow == "*" {
		if min == "*" {
			return i18n.T(ctx, "cron.every_minute")
		}
		if step, ok := strings.CutPrefix(min, "*/"); ok {
			n, ok := number(step, 1, 30)
			if !ok || 60%n != 0 {
				return ""
			}
			if n == 1 {
				return i18n.T(ctx, "cron.every_minute")
			}
			return i18n.T(ctx, "cron.every_n", "n", n)
		}
		if m, ok := number(min, 0, 59); ok {
			return i18n.T(ctx, "cron.hourly", "m", fmt.Sprintf("%02d", m))
		}
		return ""
	}

	m, okM := number(min, 0, 59)
	h, okH := number(hour, 0, 23)
	if !okM || !okH {
		return ""
	}
	at := fmt.Sprintf("%02d:%02d", h, m)
	switch {
	case dom == "*" && dow == "*":
		return i18n.T(ctx, "cron.daily", "at", at)
	case dom == "*" && dow == "1-5":
		return i18n.T(ctx, "cron.weekdays", "at", at)
	case dow == "*":
		if d, ok := number(dom, 1, 28); ok {
			return i18n.T(ctx, "cron.monthly", "d", d, "at", at)
		}
	}
	return ""
}

// number reads a plain cron number in [lo, hi]: digits only, no list, range
// or step.
func number(s string, lo, hi int) (int, bool) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}
