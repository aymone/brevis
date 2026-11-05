package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A FIELD STARTING WITH _ IS A NOTE TO WHOEVER EDITS THE FILE (_review), NOT A MESSAGE.
func TestANoteInACatalogIsNotAMessage(t *testing.T) {
	c, err := parse([]byte(`{"_review": "ignored", "hello": "Hello, {name}", "runs": {"one": "{count} run", "other": "{count} runs"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c["_review"]; ok {
		t.Error("a field starting with _ was loaded as a message")
	}
	if len(c) != 2 {
		t.Errorf("%d messages, want 2", len(c))
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
				// summary.sep.* are punctuation: Japanese and Chinese join clauses with no space at all.
				if form == "" && !strings.HasPrefix(key, "summary.sep.") {
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
