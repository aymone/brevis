package pages

import (
	"fmt"
	"strconv"
	"strings"

	"brevis.sh/brevis/web/i18n"
)

// CronMsg is a cron expression as a sentence for the app to say ("every day at
// 04:00"), or nil where the words are not sure of it: the caller shows the
// expression itself.
func CronMsg(expr string) *i18n.Msg {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil
	}
	min, hour, dom, month, dow := f[0], f[1], f[2], f[3], f[4]
	if month != "*" {
		return nil
	}

	if hour == "*" && dom == "*" && dow == "*" {
		if min == "*" {
			return ptrMsg(i18n.M("cron.every_minute"))
		}
		if step, ok := strings.CutPrefix(min, "*/"); ok {
			n, ok := number(step, 1, 30)
			if !ok || 60%n != 0 {
				return nil
			}
			if n == 1 {
				return ptrMsg(i18n.M("cron.every_minute"))
			}
			return ptrMsg(i18n.M("cron.every_n", "n", n))
		}
		if m, ok := number(min, 0, 59); ok {
			return ptrMsg(i18n.M("cron.hourly", "m", fmt.Sprintf("%02d", m)))
		}
		return nil
	}

	m, okM := number(min, 0, 59)
	h, okH := number(hour, 0, 23)
	if !okM || !okH {
		return nil
	}
	at := fmt.Sprintf("%02d:%02d", h, m)
	switch {
	case dom == "*" && dow == "*":
		return ptrMsg(i18n.M("cron.daily", "at", at))
	case dom == "*" && dow == "1-5":
		return ptrMsg(i18n.M("cron.weekdays", "at", at))
	case dow == "*":
		if d, ok := number(dom, 1, 28); ok {
			return ptrMsg(i18n.M("cron.monthly", "d", d, "at", at))
		}
	}
	return nil
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

func ptrMsg(m i18n.Msg) *i18n.Msg { return &m }

// cronWords is when a schedule runs, for the app to say: in words where they
// are sure, with the timezone after them when it is not UTC; the expression
// itself otherwise.
func cronWords(cron, tz string) i18n.Cat {
	m := CronMsg(cron)
	if m == nil {
		return plain(cron)
	}
	if zone := zoneSuffix(tz); zone != "" {
		return i18n.Cat{*m, zone}
	}
	return i18n.Cat{*m}
}
