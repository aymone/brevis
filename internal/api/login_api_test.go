package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"brevis.sh/brevis/internal/auth"
)

const wrongSentence = "Invalid username or password."

type loginRig struct {
	mux   *http.ServeMux
	gate  *auth.Gate
	calls *atomic.Int32
}

// newLoginRig counts the hashes that run; only user "ana" / "right" matches.
func newLoginRig(t *testing.T, insecure bool) loginRig {
	t.Helper()
	var calls atomic.Int32
	gate := &auth.Gate{
		Cred:     auth.Credential{User: "ana", Hash: "x", Secret: []byte("a-test-secret-with-more-than-32-bytes!!")},
		Insecure: insecure,
		Check:    func(_, pw string) bool { calls.Add(1); return pw == "right" },
	}
	mux := http.NewServeMux()
	ui := dataUI(catalogFake{})
	ui.RegistrarLoginAPI(mux, gate)
	return loginRig{mux, gate, &calls}
}

func (g loginRig) post(path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.9.9.9:1234"
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	g.mux.ServeHTTP(rec, req)
	return rec
}

func login(user, pw, next string) string {
	b, _ := json.Marshal(map[string]string{"user": user, "password": pw, "next": next})
	return string(b)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	return m
}

func TestAPILoginSetsTheSessionCookie(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		g := newLoginRig(t, insecure)
		rec := g.post("/api/login", login("ana", "right", ""), nil)
		if rec.Code != 200 || decode(t, rec)["next"] != "/" {
			t.Fatalf("status %d body %s; expected 200 {next:/}", rec.Code, rec.Body)
		}
		cs := rec.Result().Cookies()
		if len(cs) != 1 {
			t.Fatalf("%d cookies", len(cs))
		}
		c := cs[0]
		if c.Name != auth.CookieName || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure == insecure || c.Path != "/" {
			t.Errorf("cookie flags wrong (insecure=%v): %+v", insecure, c)
		}
		if d := time.Until(c.Expires); d < auth.SessionLifetime-time.Minute || d > auth.SessionLifetime {
			t.Errorf("expiry in %v", d)
		}
	}
}

func TestAPILoginNextGoesThroughTheOpenRedirectGuard(t *testing.T) {
	g := newLoginRig(t, false)
	for next, want := range map[string]string{
		"/app/runs?x=1":        "/app/runs?x=1",
		"/runs":                "/runs",
		"//evil.example":       "/",
		"https://evil.example": "/",
		`/\evil.example`:       "/",
		"/a\r\nSet-Cookie: x":  "/",
	} {
		g.gate.Limit = auth.NewLimiter() // fresh counters per vector
		rec := g.post("/api/login", login("ana", "right", next), nil)
		if got := decode(t, rec)["next"]; got != want {
			t.Errorf("next %q -> %v, want %q", next, got, want)
		}
	}
}

func TestAPILoginRefusalIsGenericAndRunsTheHashEvenForAWrongUser(t *testing.T) {
	g := newLoginRig(t, false)
	a := g.post("/api/login", login("nobody", "right", ""), nil)
	g.gate.Limit = auth.NewLimiter()
	b := g.post("/api/login", login("ana", "wrong", ""), nil)
	if a.Code != 401 || b.Code != 401 || a.Body.String() != b.Body.String() {
		t.Fatalf("wrong user %d %q vs wrong password %d %q", a.Code, a.Body, b.Code, b.Body)
	}
	if decode(t, a)["error"] != wrongSentence {
		t.Errorf("sentence = %v", decode(t, a)["error"])
	}
	if len(a.Result().Cookies()) != 0 {
		t.Error("a refusal set a cookie")
	}
	// "nobody"/"right" must NOT get in: the user is compared even when the
	// password matches, and the hash ran for both.
	if g.calls.Load() != 2 {
		t.Errorf("hash ran %d times, want 2", g.calls.Load())
	}
}

func TestAPILoginLocksTheSourceAndSkipsTheHashWhileLocked(t *testing.T) {
	g := newLoginRig(t, false)
	for i := 0; i < 4; i++ { // 3 free failures, the 4th locks
		g.post("/api/login", login("ana", "wrong", ""), nil)
	}
	n := g.calls.Load()
	rec := g.post("/api/login", login("ana", "right", ""), nil)
	body := decode(t, rec)
	if rec.Code != 429 || body["retryAfter"].(float64) < 1 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("locked: %d %s", rec.Code, rec.Body)
	}
	if g.calls.Load() != n {
		t.Error("the hash ran for a locked source")
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("the correct password signed in while locked")
	}
}

func TestAPILoginMalformedBodies(t *testing.T) {
	g := newLoginRig(t, false)
	for name, body := range map[string]string{
		"empty":     "",
		"not json":  "user=ana",
		"array":     "[]",
		"unknown":   `{"user":"ana","password":"right","admin":true}`,
		"trailing":  login("ana", "right", "") + `{"x":1}`,
		"wrongtype": `{"user":1}`,
		"oversize":  `{"user":"` + strings.Repeat("a", 9000) + `"}`,
	} {
		if rec := g.post("/api/login", body, nil); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if g.calls.Load() != 0 {
		t.Error("a malformed body ran the hash")
	}
}

func TestAPISessionEndpointsRejectCrossSiteAndNonJSON(t *testing.T) {
	g := newLoginRig(t, false)
	body := login("ana", "right", "")
	cases := map[string]struct {
		hdr  map[string]string
		want int
	}{
		"urlencoded form":                           {map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 415},
		"text/plain form":                           {map[string]string{"Content-Type": "text/plain"}, 415},
		"multipart form":                            {map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, 415},
		"no content type":                           {map[string]string{"Content-Type": ""}, 415},
		"foreign origin":                            {map[string]string{"Origin": "https://evil.example"}, 403},
		"null origin":                               {map[string]string{"Origin": "null"}, 403},
		"cross-site fetch":                          {map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		"same-site sibling":                         {map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		"cross-site beats a matching origin":        {map[string]string{"Origin": "http://example.com", "Sec-Fetch-Site": "cross-site"}, 403},
		"same-origin behind a host-rewriting proxy": {map[string]string{"Origin": "http://localhost:5210", "Sec-Fetch-Site": "same-origin"}, 200},
		"same origin ok":                            {map[string]string{"Origin": "http://example.com", "Sec-Fetch-Site": "same-origin"}, 200},
		"json with charset ok":                      {map[string]string{"Content-Type": "application/json; charset=utf-8"}, 200},
	}
	for name, c := range cases {
		for _, path := range []string{"/api/login", "/api/logout"} {
			g.gate.Limit = auth.NewLimiter()
			before := g.calls.Load()
			b := body
			rec := g.post(path, b, c.hdr)
			want := c.want
			if path == "/api/logout" && want == 200 {
				want = 204
			}
			if rec.Code != want {
				t.Errorf("%s %s: %d, want %d", name, path, rec.Code, want)
			}
			if want >= 400 && (g.calls.Load() != before || len(rec.Result().Cookies()) != 0) {
				t.Errorf("%s %s: a refused request ran the hash or touched the cookie", name, path)
			}
		}
	}
	if rec := httptest.NewRecorder(); true {
		g.mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/login", nil))
		if rec.Code != 405 {
			t.Errorf("GET /api/login = %d, want 405", rec.Code)
		}
	}
}

func TestAPILogoutClearsTheCookieDoes(t *testing.T) {
	g := newLoginRig(t, false)
	rec := g.post("/api/logout", "", nil)
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("logout = %d %q", rec.Code, rec.Body)
	}
	c := rec.Result().Cookies()[0]
	if c.Name != auth.CookieName || c.Value != "" || c.MaxAge >= 0 || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || !c.Secure {
		t.Errorf("cleared cookie: %+v", c)
	}
}
