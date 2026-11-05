package pages

import (
	"testing"
)

// A STAGE SHOWS THE NUMBERS ITS GRAPH NODE SHOWS (screen-run-v2 RV3): the
// same two, in the same order, as the same numbers -- the one-step row
// and a graph's stage pill are two drawings of one thing.
func TestStageNumbersAreTheGraphsTwo(t *testing.T) {
	got := StageNumbers(map[string]any{"objects": 1.0, "rows": 12345.0, "pages": 1.0, "detail": "/data/x.csv", "bytes": 61.0})
	if len(got) != 2 || got[0] != (StageNumber{"rows", int64(12345)}) || got[1] != (StageNumber{"pages", int64(1)}) {
		t.Errorf("numbers = %v", got)
	}
	if got := StageNumbers(map[string]any{"in": 370.0, "out": "370"}); len(got) != 2 || got[1].Value != "370" {
		t.Errorf("a string number = %v", got)
	}
	if StageNumbers(nil) != nil {
		t.Error("no numbers is not nothing")
	}
}
