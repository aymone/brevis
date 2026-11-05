package components

// ButtonKind is what a button is for, and therefore how loud it is.
type ButtonKind int

const (
	// ButtonSecondary is the default: bordered, quiet, most buttons.
	ButtonSecondary ButtonKind = iota
	// ButtonPrimary is the one thing a screen most wants done, in the accent:
	// the lime is spent here, on the open section and on the focus ring, and
	// nowhere else, so it always points at the next click. One per screen.
	ButtonPrimary
	// ButtonDanger is a verb that loses something.
	ButtonDanger
)

// ButtonAccent is ButtonPrimary by its older name: it was the workbench's
// Run, which is that screen's primary action, and the two are one kind now.
const ButtonAccent = ButtonPrimary

// ButtonSize is a button's height.
type ButtonSize int

const (
	SizeRegular ButtonSize = iota // 32px
	SizeSmall                     // 26px, in a table row or a dense toolbar
)

// SegOption is one choice in a segmented filter: a link, so the filter is a
// URL and works with no script.
type SegOption struct {
	Label, Href string
	// Count is printed beside the label when set: how many rows the choice
	// would show. A string so the caller decides "3" or "3+".
	Count   string
	Current bool
}

// Tab is one tab in a tablist.
type Tab struct {
	Key, Label string
	// Count is drawn quieter beside the label ("Logs 17"); empty draws none.
	Count    string
	Selected bool
}
