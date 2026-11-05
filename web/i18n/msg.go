package i18n

import (
	"encoding/json"
	"time"
)

// Msg is a sentence the SERVER DOES NOT SAY. The server has no language: it answers which sentence (a catalog
// key) and the data that fills it, and the app says it in the reader's language (web/spa/src/i18n.ts, useMsg).
//
//	{"k": "summary.read.many", "n": 3}                    plural: the app picks the form for 3 and fills {count}
//	{"k": "run.why.step", "a": {"step": "load"}}           named values for the {placeholders}
//	{"k": "x", "a": {"when": {"$": "dur", "ms": 90000}}}   a typed value the app formats (see Num, Dur, At)
//	{"k": "x", "a": {"inner": {"k": "y"}}}                 an argument can itself be a Msg
//
// Keys are the same catalog keys the app already reads for its own sentences (web/i18n/locales). Build one with M
// or MN, never by hand.
type Msg struct {
	K string         `json:"k"`
	N *int           `json:"n,omitempty"`
	A map[string]any `json:"a,omitempty"`
}

// M is the sentence key, with its named values as name, value pairs: M("run.why.step", "step", "load").
// A value is a string, a number, a bool, a Msg, or one of the typed values below; a pair past an odd end is ignored.
func M(key string, args ...any) Msg {
	m := Msg{K: key}
	for i := 0; i+1 < len(args); i += 2 {
		name, ok := args[i].(string)
		if !ok {
			continue
		}
		if m.A == nil {
			m.A = map[string]any{}
		}
		m.A[name] = args[i+1]
	}
	return m
}

// MN is M for a sentence that depends on a count: the app picks the plural form for count and fills {count}.
func MN(key string, count int, args ...any) Msg {
	m := M(key, args...)
	m.N = &count
	return m
}

// Num is a number the app writes with the reader's thousands separator and decimal mark.
type Num float64

func (n Num) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"$": "num", "v": float64(n)})
}

// Dur is a duration the app writes in the unit its size calls for (0.4s, 2m 05s, 1h 04m).
type Dur time.Duration

func (d Dur) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"$": "dur", "ms": time.Duration(d).Milliseconds()})
}

// At is an instant the app writes in one of the console's shapes (short, brief, day, month, weekday), 24-hour,
// in Zone (the instant's own location, an IANA name; UTC when it has none).
type At struct {
	T      time.Time
	Layout string
}

func (a At) MarshalJSON() ([]byte, error) {
	zone := a.T.Location().String()
	if zone == "" || zone == "Local" {
		zone = "UTC"
	}
	return json.Marshal(map[string]any{"$": "at", "v": a.T.UTC().Format(time.RFC3339), "layout": a.Layout, "zone": zone})
}
