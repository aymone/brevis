package pages

import "testing"

// A cron is sent as a catalog key and its values, never as a sentence; an expression the words are not sure of is
// nil, and the app shows the expression itself.
func TestACronIsSentAsAKeyAndItsValues(t *testing.T) {
	for expr, want := range map[string]string{
		"* * * * *":    "every minute",
		"*/1 * * * *":  "every minute",
		"*/15 * * * *": "every 15 minutes",
		"5 * * * *":    "hourly at :05",
		"0 4 * * *":    "every day at 04:00",
		"30 9 * * 1-5": "weekdays at 09:30",
		"0 6 15 * *":   "on day 15 of every month at 06:00",
		"*/7 * * * *":  "",
		"0 4 * 3 *":    "",
		"0 4 * * 0":    "",
		"nonsense":     "",
	} {
		m := CronMsg(expr)
		if want == "" {
			if m != nil {
				t.Errorf("%q = %+v, want none", expr, m)
			}
			continue
		}
		if m == nil || saidEN(*m) != want {
			t.Errorf("%q = %q, want %q", expr, saidEN(m), want)
		}
	}
	if got := saidEN(cronWords("0 4 * * *", "America/Sao_Paulo")); got != "every day at 04:00 (America/Sao_Paulo)" {
		t.Errorf("a zone follows the words: %q", got)
	}
	if got := saidEN(cronWords("0 4 * 3 *", "UTC")); got != "0 4 * 3 *" {
		t.Errorf("an unsure cron is the expression: %q", got)
	}
}
