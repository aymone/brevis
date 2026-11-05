package i18n

import (
	"context"
	"regexp"
	"sort"
	"testing"
)

// fixture is a catalog written for the test, so the behaviour of T and N is
// pinned without depending on what the real catalogs happen to say.
func fixture(t *testing.T) map[string]catalog {
	t.Helper()
	out := map[string]catalog{}
	for lang, raw := range map[string]string{
		"en":    `{"_review": "ignored", "hello": "Hello, {name}", "runs": {"one": "{count} run", "other": "{count} runs"}, "only_en": "English only"}`,
		"pt-BR": `{"hello": "Olá, {name}", "runs": {"one": "{count} execução", "other": "{count} execuções"}}`,
	} {
		c, err := parse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		out[lang] = c
	}
	return out
}

func TestTFillsNamedPlaceholdersInTheContextsLanguage(t *testing.T) {
	cats := fixture(t)
	pt := WithLang(context.Background(), "pt-BR")
	if got := translate(cats, pt, "hello", "name", "Ana"); got != "Olá, Ana" {
		t.Errorf("pt: %q", got)
	}
	if got := translate(cats, context.Background(), "hello", "name", "Ana"); got != "Hello, Ana" {
		t.Errorf("no language is English: %q", got)
	}
}

// ONE IS ONLY ONE. Portuguese, English and Spanish agree: 0 and 2 take the
// plural, 1 the singular.
func TestNChoosesOneOrOther(t *testing.T) {
	cats := fixture(t)
	pt := WithLang(context.Background(), "pt-BR")
	for n, want := range map[int]string{0: "0 execuções", 1: "1 execução", 2: "2 execuções"} {
		if got := plural(cats, pt, "runs", n); got != want {
			t.Errorf("N(%d) = %q, want %q", n, got, want)
		}
	}
}

// A MISSING KEY FALLS BACK TO ENGLISH, AND A KEY ENGLISH LACKS SHOWS ITSELF.
// A screen half in Portuguese and half in English is better than a blank;
// a bracketed key on the screen is a bug nobody can miss.
func TestAMissingKeyFallsBackToEnglishThenShowsItself(t *testing.T) {
	cats := fixture(t)
	pt := WithLang(context.Background(), "pt-BR")
	if got := translate(cats, pt, "only_en"); got != "English only" {
		t.Errorf("fallback = %q", got)
	}
	if got := translate(cats, pt, "nowhere"); got != "[nowhere]" {
		t.Errorf("a key nobody has = %q", got)
	}
	if _, ok := cats["en"]["_review"]; ok {
		t.Error("a field starting with _ was loaded as a message")
	}
}

// THE REAL CATALOGS ARE COMPLETE AND AGREE WITH EACH OTHER: the same keys,
// the same placeholders in every translation of a key, both plural forms
// wherever one is plural, and nothing empty. A translation that drops
// `{count}` prints a sentence with no number in it, and nothing else would
// notice.
func TestTheCatalogsAreCompleteAndConsistent(t *testing.T) {
	if len(catalogs) != len(Langs) {
		t.Fatalf("%d catalogs loaded for %d languages", len(catalogs), len(Langs))
	}
	en := catalogs["en"]
	if len(en) == 0 {
		t.Fatal("the English catalog is empty")
	}
	marks := regexp.MustCompile(`\{[a-z_]+\}`)
	set := func(s string) string {
		m := marks.FindAllString(s, -1)
		sort.Strings(m)
		return fmtSlice(m)
	}
	for _, lang := range Langs {
		c := catalogs[lang]
		for key, want := range en {
			got, ok := c[key]
			if !ok {
				t.Errorf("%s lacks %s", lang, key)
				continue
			}
			if want.plural() != got.plural() {
				t.Errorf("%s: %s is plural in one language and not in the other", lang, key)
				continue
			}
			for i, form := range got.forms() {
				if form == "" {
					t.Errorf("%s: %s is empty", lang, key)
				}
				// A form English has no counterpart for (few, many) is held to its "other". So is
				// Russian's "one": it also serves 21, 31..., so it carries {count} even where
				// English writes "1 worker".
				ref := want.forms()[min(i, 1)]
				if lang == "ru" && want.plural() {
					ref = want.forms()[1]
				}
				if set(form) != set(ref) {
					t.Errorf("%s: %s has placeholders %s, English has %s", lang, key, set(form), set(ref))
				}
			}
		}
		for key := range c {
			if _, ok := en[key]; !ok {
				t.Errorf("%s has %s, which English does not", lang, key)
			}
		}
	}
}

func fmtSlice(s []string) string {
	out := ""
	for _, v := range s {
		out += v + " "
	}
	return out
}

// THE REACT APP'S BUNDLE FALLS BACK TO ENGLISH, like the server: a key a
// language lacks is English's, a plural stays {one, other}, and a language the
// console does not speak is English.
func TestTheBundleFallsBackToEnglishLikeTheServer(t *testing.T) {
	en := Bundle("en")
	for _, lang := range Langs {
		b := Bundle(lang)
		if len(b) != len(en) {
			t.Errorf("%s: %d keys, English has %d; the fallback must fill every key", lang, len(b), len(en))
		}
	}
	if got := Bundle("xx")["nav.projects"]; got != en["nav.projects"] {
		t.Errorf("an unknown language gave %v, want English's %v", got, en["nav.projects"])
	}
	if _, ok := en["nav.catalog.attention"].(map[string]string); !ok {
		t.Errorf("a plural key is %T, want {one, other}", en["nav.catalog.attention"])
	}
	if b1, e1 := BundleJSON("pt-BR"); len(b1) == 0 || e1 == "" {
		t.Error("the encoded bundle has no body or no validator")
	}
}
