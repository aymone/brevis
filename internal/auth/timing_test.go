package auth_test

import (
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"brevis.sh/brevis/internal/auth"
)

// A wrong user must cost what a wrong password costs: the hash runs for both,
// so the response time does not reveal which names exist. Medians over
// interleaved samples, with a wide bound (an early return skips ~200 ms of
// PBKDF2, a gap of whole multiples, so noise cannot hide it).
func TestAWrongUserCostsAsMuchAsAWrongPassword(t *testing.T) {
	gate := &auth.Gate{Cred: credential(t, "operador", "senha-de-teste-longa"), Insecure: true}
	sample := func(user, pw string) time.Duration {
		start := time.Now()
		gate.SignIn(httptest.NewRecorder(), user, pw)
		return time.Since(start)
	}
	var wu, wp []time.Duration
	for i := 0; i < 5; i++ {
		wu = append(wu, sample("ninguem", "senha-de-teste-longa"))
		wp = append(wp, sample("operador", "senha-errada-qualquer"))
	}
	med := func(d []time.Duration) time.Duration {
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		return d[len(d)/2]
	}
	a, b := med(wu), med(wp)
	if a < b/2 || a > b*2 {
		t.Errorf("wrong user %v vs wrong password %v: the timing tells them apart", a, b)
	}
}
