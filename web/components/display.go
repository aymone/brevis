package components

// Stat is one number in a Stats strip.
type Stat struct {
	Label, Value string
	// Unit is printed small beside the number ("s", "GB"); Note faint, on
	// the value's line (density-base DB4).
	Unit, Note string
	// Icon is a glyph name for the label; empty draws none.
	Icon string
	// Dot is a state's color (a `bg-state-*` class) for a dot before the
	// label; empty draws none.
	Dot string
	// Bar is an optional thin bar under the value, in segments.
	Bar []BarSegment
	// Sub is faint text beside the label ("· 3 loads").
	Sub string
	// Tone is a text color class for the value, for a number that needs a
	// look (`text-state-failed`); empty is the ink.
	Tone string
	// Title is the cell's tooltip: how to read what it shows.
	Title string
}

// BarSegment is one part of a Stat's bar: its share of the whole (0..1) and
// its color class.
type BarSegment struct {
	Share float64
	Class string
}

// Def is one row of a DefinitionList.
type Def struct {
	Term, Value string
	// Mono draws the value in the machine's face: a URI, a cron, a key.
	Mono bool
}

// KVItem is one cell of a key-value grid: a machine's key, its value, and a
// note on what it means.
type KVItem struct {
	Key, Value, Note string
}
