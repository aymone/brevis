package auth_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/auth"
)

func credential(t *testing.T, user, password string) auth.Credential {
	t.Helper()
	h, err := auth.GenerateHash(password)
	if err != nil {
		t.Fatal(err)
	}
	return auth.Credential{
		User: user, Hash: h,
		Secret: []byte("a-test-secret-with-more-than-32-bytes"),
	}
}

func TestTheHashAcceptsTheRightPassword(t *testing.T) {
	h, err := auth.GenerateHash("correct-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(h, "correct-long-password") {
		t.Error("the right password was refused")
	}
	if auth.CheckPassword(h, "wrong-long-password!") {
		t.Error("the wrong password was accepted")
	}
}

// Two hashes of the SAME password have to differ: with no salt, equal hashes in
// the database
// reveal which operators chose the same password.
func TestTheHashUsesARandomSalt(t *testing.T) {
	a, _ := auth.GenerateHash("same-password-here")
	b, _ := auth.GenerateHash("same-password-here")
	if a == b {
		t.Error("two hashes of the same password came out equal -- there is no salt")
	}
}

// A malformed hash never "passes": the error path of a password checker is
// exactly where a bug becomes an open door.
func TestAMalformedHashDoesNotAuthenticate(t *testing.T) {
	for _, h := range []string{
		"", "abc", "pbkdf2-sha256$", "pbkdf2-sha256$0$c2Fs$Y2hhdmU",
		"pbkdf2-sha256$600000$!!!$!!!", "md5$1$x$y",
		"pbkdf2-sha256$600000$c2Fs$", // empty key
	} {
		if auth.CheckPassword(h, "anything") {
			t.Errorf("a malformed hash authenticated: %q", h)
		}
		if auth.CheckPassword(h, "") {
			t.Errorf("malformed hash %q authenticated with an empty password", h)
		}
	}
}

// This is the test that stands for the incident: in dev, an anonymous POST to
// /workflows/<slug>/trigger answered 303 and fired a `dbt build` that writes into
// the data warehouse.
func TestAnAnonymousTriggerIsBlocked(t *testing.T) {
	gate := &auth.Gate{
		Cred: credential(t, "operator", "long-test-password"),
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("the anonymous request reached the protected handler")
			w.WriteHeader(http.StatusOK)
		}),
	}

	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, httptest.NewRequest("POST", "/workflows/id_verification/trigger", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; expected 401", rec.Code)
	}
}

// The redirect has to write the parameter the login screen READS, and it has to
// survive a round trip through the URL.
//
// It did not. The redirect wrote `?de=` and internal/api asked for `next`, so an
// operator who followed a deep link while logged out signed in and landed on
// `/`, with nothing anywhere saying why. Asserting only that the Location
// mentions "/runs" was what let it through: both halves have to be checked, and
// the value has to be checked DECODED -- the original query string carries a
// `?` and a `=` of its own.
func TestAnAnonymousGETGoesToTheLoginCarryingWhereItWasGoing(t *testing.T) {
	gate := &auth.Gate{
		Cred: credential(t, "operator", "long-test-password"),
		Next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	}

	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, httptest.NewRequest("GET", "/runs?page=2&state=failed", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; expected 303", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location is not a URL: %v", err)
	}
	if loc.Path != "/login" {
		t.Errorf("Location path = %q, want /login", loc.Path)
	}

	// The name is the constant, so this test moves with the code rather than
	// pinning a word.
	got := loc.Query().Get(auth.NextParam)
	if want := "/runs?page=2&state=failed"; got != want {
		t.Errorf("%s = %q, want %q -- the whole query string has to survive, or "+
			"signing in drops the filter the operator had", auth.NextParam, got, want)
	}

	// And what comes back out is what the login screen would use.
	if target := auth.Target(got); target != "/runs?page=2&state=failed" {
		t.Errorf("Target(%q) = %q; the two ends do not agree", got, target)
	}
}

// Kubernetes's probes have to get through: a /health asking for a password takes
// the pod in a loop, and the operator looks for the problem in the wrong place.
func TestProbesAndAssetsPassWithNoSession(t *testing.T) {
	var arrived []string
	gate := &auth.Gate{
		Cred: credential(t, "operator", "long-test-password"),
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			arrived = append(arrived, r.URL.Path)
		}),
	}
	for _, path := range []string{"/health", "/ready", "/assets/app.css", "/assets/fonts/x.woff2"} {
		gate.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	if len(arrived) != 4 {
		t.Errorf("%v got through; expected the four free routes", arrived)
	}
}

func TestAValidSessionPassesAndCarriesTheUser(t *testing.T) {
	cred := credential(t, "operator", "long-test-password")
	var seen string
	gate := &auth.Gate{
		Cred: cred,
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = auth.OperatorOf(r.Context())
		}),
	}

	login := httptest.NewRecorder()
	if !gate.SignIn(login, "operator", "long-test-password") {
		t.Fatal("the right credential was refused")
	}
	cookie := login.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/runs", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if seen != "operator" {
		t.Errorf("operator in the context = %q; want %q", seen, "operator")
	}
	if rec.Code == http.StatusSeeOther {
		t.Error("the valid session was sent to the login")
	}
}

// A forged cookie must not count. If the signature were not checked, the
// client would write its own username and its own expiry.
func TestAForgedCookieDoesNotGetIn(t *testing.T) {
	cred := credential(t, "operator", "long-test-password")
	gate := &auth.Gate{
		Cred: cred,
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("a forged cookie got past the gate")
			w.WriteHeader(http.StatusOK)
		}),
	}
	for _, value := range []string{
		"operator|99999999999|any-signature",
		"operator|99999999999|",
		"operator|99999999999",
		"other|99999999999|",
		"",
	} {
		req := httptest.NewRequest("GET", "/runs", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: value})
		gate.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// Changing the secret invalidates the sessions -- and it is the emergency lever.
func TestChangingTheSecretInvalidatesSessions(t *testing.T) {
	cred := credential(t, "operator", "long-test-password")
	issuer := &auth.Gate{Cred: cred, Next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	rec := httptest.NewRecorder()
	issuer.SignIn(rec, "operator", "long-test-password")
	cookie := rec.Result().Cookies()[0]

	fresh := cred
	fresh.Secret = []byte("another-completely-different-secret-32")
	gate := &auth.Gate{
		Cred: fresh,
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("the session survived the secret being changed")
		}),
	}
	req := httptest.NewRequest("GET", "/runs", nil)
	req.AddCookie(cookie)
	gate.ServeHTTP(httptest.NewRecorder(), req)
}

// `/login?next=https://malicious` must not send the operator off-site.
func TestAnExternalDestinationIsDiscarded(t *testing.T) {
	for raw, expected := range map[string]string{
		"https://malicioso.example": "/",
		"//malicioso.example":       "/",
		"/runs?page=2":              "/runs?page=2",
		"":                          "/",
	} {
		if d := auth.Target(raw); d != expected {
			t.Errorf("Target(%q) = %q; want %q", raw, d, expected)
		}
	}
}

func TestAHalfCredentialIsRefused(t *testing.T) {
	if err := (auth.Credential{User: "operator"}).Validate(); err == nil {
		t.Error("a user with no hash was accepted -- the door would stay open believing it had shut")
	}
	if err := (auth.Credential{Hash: "pbkdf2-sha256$1$a$b"}).Validate(); err == nil {
		t.Error("a hash with no user was accepted")
	}
	short := auth.Credential{User: "o", Hash: "pbkdf2-sha256$1$a$b", Secret: []byte("short")}
	if err := short.Validate(); err == nil {
		t.Error("a secret that is too short was accepted")
	}
}

// `?next=` CANNOT LEAVE THIS SITE, including by the spelling a browser
// normalizes on the way out.
//
// `escapeTarget` refused `//evil.com` and `https://evil.com` and let
// `/\evil.com` through. It looks like a path and it is not one: a backslash
// is a SLASH in a URL, so the browser reads `//evil.com` and the host is
// evil.com. Measured, not reasoned about -- `url.Parse` on the normalized
// string reports `Host: evil.com`.
//
// What that buys an attacker is the thing this function's own comment says
// it exists to stop: our own login screen, at our own domain, handing the
// operator who just authenticated to somebody else's page.
//
// Found by gosec (G710), which the CI ran against `./sdk/...` only — so the
// console it was pointed away from is where it was true.
func TestTheNextParameterCannotLeaveThisSite(t *testing.T) {
	for _, away := range []string{
		`/\evil.com`,
		`/\\evil.com`,
		`/\/evil.com`,
		`//evil.com`,
		`///evil.com`,
		`https://evil.com`,
		`http:/evil.com`,
		`/%0d%0aX-Injected:+1`,
		"/\tevil.com",
		"/\nevil.com",
		"\\\\evil.com",
		`javascript:alert(1)`,
	} {
		if got := auth.Target(away); strings.ContainsAny(got, `\`) || got != "/" && !strings.HasPrefix(got, "/") {
			t.Errorf("Target(%q) = %q, which can leave this site", away, got)
		}
		// AND A BROWSER'S READING OF IT, which is the only reading that
		// matters: normalize the backslashes the way a URL parser does and
		// ask whether a host appeared.
		u, err := url.Parse(strings.ReplaceAll(auth.Target(away), `\`, "/"))
		if err != nil {
			t.Errorf("Target(%q) = %q, which does not parse: %v", away, auth.Target(away), err)
			continue
		}
		if u.Host != "" || u.Scheme != "" {
			t.Errorf("Target(%q) = %q, which a browser reads as host %q scheme %q",
				away, auth.Target(away), u.Host, u.Scheme)
		}
	}
}

// AND A REAL DESTINATION STILL SURVIVES, which is the whole point of
// carrying `?next=` at all: somebody who was deep-linked to a filtered list
// and had to sign in lands back on it.
func TestTheNextParameterKeepsAnOrdinaryPath(t *testing.T) {
	for _, keep := range []string{
		"/runs",
		"/runs?status=failed&workflow=daily_sales",
		"/data/target?u=postgres%3A%2F%2Fwarehouse%2Fsales%2Fdaily",
		"/sql",
	} {
		if got := auth.Target(keep); got != keep {
			t.Errorf("Target(%q) = %q, and the operator loses where they were going", keep, got)
		}
	}
}

// UPGRADING SIGNS NOBODY OUT: a session cookie written by a release before 0.43, under the old name, is still honored
// until it expires, and signing out ends it too.
func TestASessionUnderTheLegacyCookieNameStillSignsIn(t *testing.T) {
	cred := credential(t, "operator", "long-test-password")
	issuer := &auth.Gate{Cred: cred, Next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	rec := httptest.NewRecorder()
	issuer.SignIn(rec, "operator", "long-test-password")
	signed := rec.Result().Cookies()[0]
	if signed.Name != auth.CookieName {
		t.Fatalf("a new session is written as %q, want %q", signed.Name, auth.CookieName)
	}

	reached := false
	gate := &auth.Gate{Cred: cred, Next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })}
	req := httptest.NewRequest("GET", "/runs", nil)
	req.AddCookie(&http.Cookie{Name: auth.LegacyCookieName, Value: signed.Value})
	gate.ServeHTTP(httptest.NewRecorder(), req)
	if !reached {
		t.Error("a session under the legacy cookie name was refused")
	}

	out := httptest.NewRecorder()
	gate.SignOut(out)
	cleared := map[string]bool{}
	for _, c := range out.Result().Cookies() {
		cleared[c.Name] = c.MaxAge < 0
	}
	if !cleared[auth.CookieName] || !cleared[auth.LegacyCookieName] {
		t.Errorf("signing out clears %v, want both names", cleared)
	}
}
