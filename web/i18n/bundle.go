package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Bundle is every message the console has in lang, as the React app reads it:
// a string, or {"one","other"} (plus "few" and "many" where the language has
// them) for a plural. English fills what lang lacks, so
// the app does no fallback of its own and a half-translated language reads the
// same there as on the server pages (lookup does the same).
func Bundle(lang string) map[string]any {
	lang = supported(lang)
	out := make(map[string]any, len(catalogs["en"]))
	for _, l := range []string{"en", lang} {
		for key, m := range catalogs[l] {
			if m.plural() {
				forms := map[string]string{"one": m.one, "other": m.other}
				if m.few != "" {
					forms["few"] = m.few
				}
				if m.many != "" {
					forms["many"] = m.many
				}
				out[key] = forms
			} else {
				out[key] = m.text
			}
		}
	}
	return out
}

type encoded struct {
	body []byte
	etag string
}

var (
	bundleMu    sync.Mutex
	bundleCache = map[string]encoded{}
)

// BundleJSON is Bundle already encoded, with a validator. The catalogs are
// fixed at start, so each language is encoded once.
func BundleJSON(lang string) (body []byte, etag string) {
	lang = supported(lang)
	bundleMu.Lock()
	defer bundleMu.Unlock()
	if e, ok := bundleCache[lang]; ok {
		return e.body, e.etag
	}
	b, _ := json.Marshal(map[string]any{"lang": lang, "messages": Bundle(lang)})
	sum := sha256.Sum256(b)
	e := encoded{body: b, etag: `"` + hex.EncodeToString(sum[:16]) + `"`}
	bundleCache[lang] = e
	return e.body, e.etag
}
