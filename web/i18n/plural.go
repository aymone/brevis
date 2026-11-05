package i18n

// category is the plural category of n in lang: one, few, many or other.
//
// Most of the console's languages need only one (n == 1) and other. Russian
// needs more: "1 запуск", "2 запуска", "5 запусков", "21 запуск". A language
// that needs a rule of its own is added here AND in web/spa/src/plural.ts, which
// must agree (plural_test.go and plural.test.ts hold the same table).
func category(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	if lang == "ru" {
		switch m10, m100 := n%10, n%100; {
		case m10 == 1 && m100 != 11:
			return "one"
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return "few"
		default:
			return "many"
		}
	}
	if n == 1 {
		return "one"
	}
	return "other"
}
