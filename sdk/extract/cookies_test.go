package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk/internal/core"
)

// paddedJWT imitates a NextAuth token: base64 with "=" padding, which is the
// character that breaks whoever splits name=value on every "=".
const paddedJWT = "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..QUJDRA=="

// TestTheCallersCookieArrivesWhole: the consumer wrote 36 lines to move
// Set-Cookie onto the header, and the trap was cutting the JWT at the second
// "=". Here the cookie has to arrive identical to what the caller passed.
func TestTheCallersCookieArrivesWhole(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session-token")
		if err != nil {
			http.Error(w, "sem cookie", http.StatusUnauthorized)
			return
		}
		received = c.Value
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	}))
	defer srv.Close()

	drain(t, core.Source{
		URL:    srv.URL,
		Header: map[string][]string{"Cookie": {"session-token=" + paddedJWT}},
	})

	if received != paddedJWT {
		t.Errorf("the server received %q, the caller sent %q", received, paddedJWT)
	}
}

// TestARenewedCookieSurvivesIntoTheNextPage: it is exactly what the consumer's
// cookie.go did by hand. If the jar disappears, page 2 goes with the old
// token.
func TestARenewedCookieSurvivesIntoTheNextPage(t *testing.T) {
	var seenValues []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session-token")
		if err != nil {
			http.Error(w, "sem cookie", http.StatusUnauthorized)
			return
		}
		seenValues = append(seenValues, c.Value)

		if len(seenValues) == 1 {
			// the API renews the session in the middle of the walk
			http.SetCookie(w, &http.Cookie{Name: "session-token", Value: "renovado==", Path: "/"})
			_, _ = fmt.Fprint(w, `{"results":[{"n":1}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"results":[]}`)
	}))
	defer srv.Close()

	drain(t, core.Source{
		URL:     srv.URL,
		PageKey: "page",
		DataKey: "results",
		Header:  map[string][]string{"Cookie": {"session-token=" + paddedJWT}},
	})

	if len(seenValues) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(seenValues))
	}
	if seenValues[1] != "renovado==" {
		t.Errorf("page 2 went with %q; page 1's Set-Cookie did not take", seenValues[1])
	}
}

// TestTheCookieDoesNotGoTwice: the caller's header and the jar are the same
// thing. If both go, the server receives two values for the same name and
// escolhe um deles -- silenciosamente o errado.
func TestTheCookieDoesNotGoTwice(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw = r.Header.Get("Cookie")
		http.SetCookie(w, &http.Cookie{Name: "session-token", Value: "renovado==", Path: "/"})
		if strings.Contains(raw, "renovado") {
			_, _ = fmt.Fprint(w, `{"results":[]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"results":[{"n":1}]}`)
	}))
	defer srv.Close()

	drain(t, core.Source{
		URL:     srv.URL,
		PageKey: "page",
		DataKey: "results",
		Header:  map[string][]string{"Cookie": {"session-token=" + paddedJWT}},
	})

	if n := strings.Count(raw, "session-token="); n != 1 {
		t.Errorf("session-token appears %d times in the header: %q", n, raw)
	}
}

// TestAMalformedCookieFailsEarly: an invalid cookie header has to complain at
// assembly time, not become a 401 at the server.
func TestAMalformedCookieFailsEarly(t *testing.T) {
	_, err := JSON(context.Background(), core.Source{
		URL:    "http://exemplo.invalido",
		Header: map[string][]string{"Cookie": {"isso nao e um cookie"}},
	}, nil)
	if err == nil {
		t.Fatal("an invalid cookie got through")
	}
	if !strings.Contains(err.Error(), "Cookie") {
		t.Errorf("the error does not say the problem is the Cookie: %v", err)
	}
}

// TestTheCallersHeaderIsNotMutated: the header belongs to the consumer, and they
// may reuse the same map in another pipeline.
func TestTheCallersHeaderIsNotMutated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	}))
	defer srv.Close()

	h := map[string][]string{"Cookie": {"session-token=" + paddedJWT}}
	drain(t, core.Source{URL: srv.URL, Header: h})

	if got := http.Header(h).Get("Cookie"); got != "session-token="+paddedJWT {
		t.Errorf("the SDK touched the caller's header: %q", got)
	}
}

func drain(t *testing.T, s core.Source) {
	t.Helper()
	seq, err := JSON(context.Background(), s, nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	for _, err := range seq {
		if err != nil {
			t.Fatalf("iterando: %v", err)
		}
	}
}

// TestASecurePrefixedCookieDoesNotVanishSilently: NextAuth's real cookie name
// starts with __Secure-, which in a browser spec only applies over https. If
// the jar applied that rule, the cookie would vanish before leaving -- and the
// SDK would fail with a 401 without ever saying it discarded the credential.
//
// The stdlib's jar does not apply the prefix rule. This test exists for the day
// that changes.
func TestASecurePrefixedCookieDoesNotVanishSilently(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Cookie")
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	}))
	defer srv.Close()

	drain(t, core.Source{URL: srv.URL, Auth: &core.Credential{
		Value: func(context.Context) (string, error) {
			return "__Secure-authjs.session-token=abc==", nil
		},
		Apply: core.AsCookie,
	}})

	if seen == "" {
		t.Fatal("the __Secure- cookie vanished before reaching the server")
	}
}
