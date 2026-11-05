package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"brevis.sh/brevis/sdk/internal/core"
)

// TestAuthAppliesTheSecret: the four ways of putting the credential on the
// request.
func TestAuthAppliesTheSecret(t *testing.T) {
	cases := []struct {
		name     string
		applyTo  core.Applier
		secret   string
		header   string
		expected string
	}{
		{"bearer", core.AsBearer, "abc", "Authorization", "Bearer abc"},
		{"whole cookie", core.AsCookie, "session=abc==", "Cookie", "session=abc=="},
		{"named cookie", core.AsCookieNamed("session"), "abc==", "Cookie", "session=abc=="},
		{"own header", core.AsHeader("X-API-Key"), "abc", "X-API-Key", "abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seen string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Header.Get(c.header)
				_, _ = fmt.Fprint(w, `{"ok":1}`)
			}))
			defer srv.Close()

			secret := c.secret
			drain(t, core.Source{URL: srv.URL, Auth: &core.Credential{
				Value: func(context.Context) (string, error) { return secret, nil },
				Apply: c.applyTo,
			}})

			if seen != c.expected {
				t.Errorf("%s = %q, want %q", c.header, seen, c.expected)
			}
		})
	}
}

// TestRefreshRenewsTheCookieForThePages: the whole mechanism. The renewal
// reissues the cookie, the jar absorbs it, and the following pages go with the
// new one -- with nothing written anywhere.
func TestRefreshRenewsTheCookieForThePages(t *testing.T) {
	var mu sync.Mutex
	var data []string

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/session", func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("session"); c == nil || c.Value != "pasted==" {
			http.Error(w, "no pasted cookie", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "renewed==", Path: "/"})
		_, _ = fmt.Fprint(w, `{"expires":"`+time.Now().Add(30*24*time.Hour).Format(time.RFC3339)+`"}`)
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("session")
		mu.Lock()
		if c != nil {
			data = append(data, c.Value)
		}
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var stats core.Stats
	drain(t, core.Source{
		URL:   srv.URL + "/data",
		Stats: &stats,
		Auth: &core.Credential{
			Value: func(context.Context) (string, error) { return "session=pasted==", nil },
			Apply: core.AsCookie,
			Refresh: &core.Refresh{
				URL:       srv.URL + "/auth/session",
				ExpiresAt: core.JSONField("expires"),
			},
		},
	})

	if len(data) != 1 || data[0] != "renewed==" {
		t.Errorf("the page went with %v; expected the cookie reissued by the renewal", data)
	}
	if stats.CredentialExpiry.IsZero() {
		t.Error("Stats.CredentialExpiry stayed zeroed; the expiry never reached whoever observes it")
	}
}

// TestARefreshThatFailsStopsTheRun: carrying on after a refused refresh sends
// every page with a credential the API just denied, and the error
// shows up blaming the data endpoint.
func TestARefreshThatFailsStopsTheRun(t *testing.T) {
	var askedForData atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/session", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "session expired", http.StatusUnauthorized)
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		askedForData.Store(true)
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := JSON(context.Background(), core.Source{
		URL: srv.URL + "/data",
		Auth: &core.Credential{
			Value:   core.FromEnv("PATH"), // any env var that exists
			Apply:   core.AsBearer,
			Refresh: &core.Refresh{URL: srv.URL + "/auth/session"},
		},
	}, nil)

	if err == nil {
		t.Fatal("a refused renewal went unnoticed")
	}
	if !strings.Contains(err.Error(), "refresh") || !strings.Contains(err.Error(), "401") {
		t.Errorf("the error does not say it was the renewal: %v", err)
	}
	if askedForData.Load() {
		t.Error("asked for data after the renewal failed")
	}
}

// TestTTLCachesTheLogin: the ana API blocks on authentication FREQUENCY, not on
// requests. Without a cache, every pipeline in the process makes a fresh
// login.
func TestTTLCachesTheLogin(t *testing.T) {
	var logins atomic.Int32
	cred := &core.Credential{
		TTL:   time.Hour,
		Apply: core.AsBearer,
		Value: func(context.Context) (string, error) {
			logins.Add(1)
			return "token", nil
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cred.Get(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if n := logins.Load(); n != 1 {
		t.Errorf("%d logins for 20 concurrent calls; the TTL should keep it at 1", n)
	}
}

// TestWithoutTTLItDoesNotCache: a zeroed TTL has to go on calling Value, or the
// field would be caching without anybody asking.
func TestWithoutTTLItDoesNotCache(t *testing.T) {
	var n int
	cred := &core.Credential{
		Apply: core.AsBearer,
		Value: func(context.Context) (string, error) { n++; return "t", nil },
	}
	for i := 0; i < 3; i++ {
		_, _ = cred.Get(context.Background())
	}
	if n != 3 {
		t.Errorf("Value called %d times without a TTL, want 3", n)
	}
}

// TestAuthRefusesConfigurationThatCannotWork: each of these would pass and
// become a 401, or a written field that does nothing.
func TestAuthRefusesConfigurationThatCannotWork(t *testing.T) {
	cases := []struct {
		name string
		cred *core.Credential
		says string
	}{
		{"without Value", &core.Credential{Apply: core.AsBearer}, "Value"},
		{"without Apply", &core.Credential{Value: core.FromEnv("PATH")}, "Apply"},
		{"Refresh without URL", &core.Credential{
			Value: core.FromEnv("PATH"), Apply: core.AsBearer,
			Refresh: &core.Refresh{},
		}, "URL"},
		{"WarnAfter without ExpiresAt", &core.Credential{
			Value: core.FromEnv("PATH"), Apply: core.AsBearer,
			Refresh: &core.Refresh{URL: "http://x", WarnAfter: time.Hour},
		}, "ExpiresAt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := JSON(context.Background(), core.Source{URL: "http://x", Auth: c.cred}, nil)
			if err == nil {
				t.Fatal("passed")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the error does not point to %q: %v", c.says, err)
			}
		})
	}
}

// TestAMissingEnvVarSaysItsName: otherwise it becomes an empty header and a 401 blaming the API.
func TestAMissingEnvVarSaysItsName(t *testing.T) {
	_, err := core.FromEnv("BREVIS_ENV_THAT_DOES_NOT_EXIST")(context.Background())
	if err == nil || !strings.Contains(err.Error(), "BREVIS_ENV_THAT_DOES_NOT_EXIST") {
		t.Errorf("the error does not name the variable: %v", err)
	}
}

// TestAuthDoesNotMutateTheCallersHeader: the map belongs to the consumer and
// must not come back
// carrying the secret.
func TestAuthDoesNotMutateTheCallersHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	}))
	defer srv.Close()

	h := map[string][]string{"X-Trace": {"1"}}
	drain(t, core.Source{URL: srv.URL, Header: h, Auth: &core.Credential{
		Value: func(context.Context) (string, error) { return "secret", nil },
		Apply: core.AsBearer,
	}})

	if _, has := h["Authorization"]; has {
		t.Errorf("the secret stayed in the caller's header: %v", h)
	}
}

// TestRefreshRetries: the pages get three attempts; the refresh had one. A
// network blip on the refresh killed the whole run while the
// same blip on the data endpoint cost one retry.
func TestRefreshRetries(t *testing.T) {
	var attempts atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"ok":1}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	drain(t, core.Source{
		URL: srv.URL + "/data",
		RetryConfig: &core.RetryConfig{
			MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
		},
		Auth: &core.Credential{
			Value:   func(context.Context) (string, error) { return "t", nil },
			Apply:   core.AsBearer,
			Refresh: &core.Refresh{URL: srv.URL + "/auth"},
		},
	})

	if n := attempts.Load(); n != 3 {
		t.Errorf("the renewal tried %d times, want 3", n)
	}
}
