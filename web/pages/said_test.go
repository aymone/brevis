package pages

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"brevis.sh/brevis/web/i18n"
)

// saidEN is what the app would say for a value the server sends, in English: the catalog's text filled with the
// message's values. It stands in for the app's renderer (web/spa/src/i18n.ts) so a test reads the sentence.
func saidEN(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case i18n.Cat:
		var b strings.Builder
		for _, p := range x {
			b.WriteString(saidEN(p))
		}
		return b.String()
	case *i18n.Msg:
		if x == nil {
			return ""
		}
		return saidEN(*x)
	case i18n.Num:
		return thousands(int64(x))
	case i18n.Msg:
		text := catalogEN(x.K, x.N)
		if x.N != nil {
			text = strings.ReplaceAll(text, "{count}", thousands(int64(*x.N)))
		}
		for name, a := range x.A {
			text = strings.ReplaceAll(text, "{"+name+"}", saidEN(a))
		}
		return text
	}
	return fmt.Sprint(v)
}

func thousands(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// catalogEN is a key's English text; a plural has its two forms, "one" for 1 and "other" else.
func catalogEN(key string, n *int) string {
	raw, err := os.ReadFile("../i18n/locales/en.json")
	if err != nil {
		panic(err)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		panic(err)
	}
	var s string
	if json.Unmarshal(all[key], &s) == nil {
		return s
	}
	var forms map[string]string
	if err := json.Unmarshal(all[key], &forms); err != nil {
		return "[" + key + "]"
	}
	if n != nil && *n == 1 {
		return forms["one"]
	}
	return forms["other"]
}
