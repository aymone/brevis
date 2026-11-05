package i18n

import (
	"context"
	"testing"
)

// The same table is in web/spa/src/plural.test.ts: the server and the app must pick the same form.
func TestCategory(t *testing.T) {
	for _, c := range []struct {
		lang string
		n    int
		want string
	}{
		{"en", 0, "other"}, {"en", 1, "one"}, {"en", 2, "other"}, {"pt-BR", 21, "other"}, {"ja", 1, "one"},
		{"ru", 0, "many"}, {"ru", 1, "one"}, {"ru", 2, "few"}, {"ru", 4, "few"}, {"ru", 5, "many"},
		{"ru", 11, "many"}, {"ru", 12, "many"}, {"ru", 14, "many"}, {"ru", 21, "one"}, {"ru", 22, "few"},
		{"ru", 25, "many"}, {"ru", 101, "one"}, {"ru", 111, "many"}, {"ru", 112, "many"}, {"ru", 1002, "few"},
	} {
		if got := category(c.lang, c.n); got != c.want {
			t.Errorf("%s %d = %s; want %s", c.lang, c.n, got, c.want)
		}
	}
}

// THE CASE THAT STARTED THIS: Russian counts 21 like 1, and 2 to 4 take their own form.
func TestRussianCountsReadRight(t *testing.T) {
	ctx := WithLang(context.Background(), "ru")
	for n, want := range map[int]string{1: "из 1 назначения", 2: "из 2 назначений", 21: "из 21 назначения", 25: "из 25 назначений", 111: "из 111 назначений"} {
		if got := N(ctx, "catalog.count", n); got != want {
			t.Errorf("catalog.count %d = %q; want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{1: "1 запуск", 3: "3 запуска", 5: "5 запусков", 22: "22 запуска"} {
		if got := N(ctx, "calendar.runs", n); got != want {
			t.Errorf("calendar.runs %d = %q; want %q", n, got, want)
		}
	}
	if forms := Bundle("ru")["calendar.runs"].(map[string]string); forms["few"] == "" || forms["many"] == "" {
		t.Errorf("the app's bundle lacks few/many: %v", forms)
	}
}
