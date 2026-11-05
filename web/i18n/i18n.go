// Package i18n holds the console's catalogs and says NOTHING with them: THE SERVER HAS NO LANGUAGE.
//
// A message is a semantic key ("nav.overview") with a text per language, in locales/*.json, compiled into the
// binary. Every language has the same keys, which a test holds. The catalogs reach the one place that speaks,
// the React app (web/spa), as /api/messages (Bundle, BundleJSON); the server's own answers are data and codes:
//
//   - a sentence it must hand over is a Msg (msg.go) or a Cat (cat.go): a catalog key and the data that fills it,
//     which the app says in the reader's language;
//   - a failure is an error code (internal/api/apierror.go), never prose.
//
// What this package still does is negotiate which language the reader asked for (Negotiate, Remember) and
// serve the catalogs. MAKING A SCREEN TRANSLATABLE: say it in the app with t("screen.what") or n("screen.what",
// count), add the key to all the files in locales/ with the same placeholders (English is the default and the
// fallback; a key it lacks shows as [key]), and let keys_test.go and i18n_test.go hold the rest.
//
// Product and engine terms are never translated (BigQuery, dbt, gateway, slot, dead letter, Workflows...);
// keys_test.go holds that, and that no key is missing or unused.
//
// NO LIBRARY. Placeholders are named and plurals are {one, other} (plus few and many where a language has them,
// web/spa/src/plural.ts): a dependency would add packages to an engine whose size a gate watches.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Langs are the languages the console speaks, English first: it is the
// default and the fallback.
var Langs = []string{"en", "es", "pt-BR", "fr", "de", "zh-CN", "ja", "ru", "ko", "hi", "id"}

//go:embed locales/*.json
var locales embed.FS

// message is one key's text, or its plural forms: one and other always, few
// and many only in a language whose rule has them (plural.go).
type message struct {
	text                  string
	one, few, many, other string
	// isPlural is explicit because a plain message may be EMPTY: Japanese and Chinese join the clauses of a summary
	// with no space at all (summary.sep.space).
	isPlural bool
}

func (m message) plural() bool { return m.isPlural }

func (m message) forms() []string {
	if !m.plural() {
		return []string{m.text}
	}
	out := []string{m.one, m.other}
	for _, f := range []string{m.few, m.many} {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

type catalog map[string]message

// catalogs are read once, at start: a catalog that does not parse stops the
// process at boot rather than leaving a page half-translated.
var catalogs = func() map[string]catalog {
	out := map[string]catalog{}
	for _, lang := range Langs {
		raw, err := locales.ReadFile("locales/" + lang + ".json")
		if err != nil {
			panic(fmt.Sprintf("i18n: %s: %v", lang, err))
		}
		c, err := parse(raw)
		if err != nil {
			panic(fmt.Sprintf("i18n: %s: %v", lang, err))
		}
		out[lang] = c
	}
	return out
}()

// parse reads one catalog. A key starting with `_` is a note to whoever
// edits the file (`_review`), not a message.
func parse(raw []byte) (catalog, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c := catalog{}
	for key, v := range doc {
		if strings.HasPrefix(key, "_") {
			continue
		}
		var text string
		if err := json.Unmarshal(v, &text); err == nil {
			c[key] = message{text: text}
			continue
		}
		var forms struct{ One, Few, Many, Other string }
		if err := json.Unmarshal(v, &forms); err != nil || forms.One == "" || forms.Other == "" {
			return nil, fmt.Errorf("%s is neither a string nor {\"one\", \"other\"} (and optionally \"few\", \"many\")", key)
		}
		c[key] = message{one: forms.One, few: forms.Few, many: forms.Many, other: forms.Other, isPlural: true}
	}
	return c, nil
}

func supported(lang string) string {
	for _, l := range Langs {
		if l == lang {
			return l
		}
	}
	return "en"
}
