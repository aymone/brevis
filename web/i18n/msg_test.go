package i18n

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAMsgIsAKeyAndTheDataThatFillsIt(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 5, 0, 0, time.UTC)
	for name, c := range map[string]struct {
		msg  Msg
		want string
	}{
		"a key alone":     {M("run.why.run"), `{"k":"run.why.run"}`},
		"named values":    {M("run.why.step", "step", "load", "code", 2), `{"k":"run.why.step","a":{"code":2,"step":"load"}}`},
		"a plural":        {MN("summary.read.many", 3), `{"k":"summary.read.many","n":3}`},
		"zero is a count": {MN("summary.read.many", 0), `{"k":"summary.read.many","n":0}`},
		"typed values": {M("x", "n", Num(1200), "d", Dur(90*time.Second), "t", At{at, "short"}),
			`{"k":"x","a":{"d":{"$":"dur","ms":90000},"n":{"$":"num","v":1200},"t":{"$":"at","layout":"short","v":"2026-10-02T09:05:00Z","zone":"UTC"}}}`},
		"nested":  {M("outer", "inner", M("inner.key")), `{"k":"outer","a":{"inner":{"k":"inner.key"}}}`},
		"odd end": {M("k", "a"), `{"k":"k"}`},
	} {
		b, err := json.Marshal(c.msg)
		if err != nil || string(b) != c.want {
			t.Errorf("%s: %s (%v), want %s", name, b, err, c.want)
		}
	}
}
