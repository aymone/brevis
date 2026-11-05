package workflow

import (
	"errors"
	"reflect"
	"testing"
)

// A REFUSED VALUE IS A CODE WITH ITS DATA, and Error() is still the sentence the CLI prints.
func TestParamErrorsCarryCodeDataAndTheSameSentence(t *testing.T) {
	for _, c := range []struct {
		p    Param
		in   string
		want ParamError
		text string
	}{
		{Param{Type: ParamBool}, "x", ParamError{Code: ParamNotBoolean, Value: "x"}, `"x" is not a boolean (use true or false)`},
		{Param{Type: ParamInteger}, "x", ParamError{Code: ParamNotInteger, Value: "x"}, `"x" is not an integer`},
		{Param{Type: ParamString, Enum: []string{"a", "b"}}, "x", ParamError{Code: ParamNotOneOf, Value: "x", Allowed: []string{"a", "b"}}, `"x" is not one of the accepted values (a, b)`},
		{Param{Type: ParamString, Pattern: "^a$"}, "x", ParamError{Code: ParamNoMatch, Value: "x", Pattern: "^a$"}, `"x" does not match the pattern "^a$"`},
		{Param{Type: ParamString}, "a;b", ParamError{Code: ParamUnsafeChars, Value: "a;b"}, "\"a;b\" has a character the shell interprets; declare a `pattern` on the param if the value genuinely needs it"},
		{Param{Type: "list|integer"}, "1,x", ParamError{Code: ParamNotInteger, Value: "x", Item: 2}, `item 2: "x" is not an integer`},
		{Param{Type: "list|string"}, "a,,b", ParamError{Code: ParamItemEmpty, Item: 2}, "item 2 is empty; a list is items separated by commas, with no empty ones"},
		{Param{Type: "list|string"}, "a,a", ParamError{Code: ParamItemDuplicate, Value: "a"}, `item "a" appears more than once`},
	} {
		err := c.p.Accepts(c.in)
		var got *ParamError
		if !errors.As(err, &got) || !reflect.DeepEqual(*got, c.want) || err.Error() != c.text {
			t.Errorf("%v %q: got %#v / %q, want %#v / %q", c.p.Type, c.in, got, err, c.want, c.text)
		}
	}
}
