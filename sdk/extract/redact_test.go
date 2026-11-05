package extract

import (
	"strings"
	"testing"
)

// The redacted URL appears on every extract log line and in every error message.
// A leak here is a live credential in a log aggregator plenty of people read.
func TestRedactLetsNoSecretThrough(t *testing.T) {
	cases := []string{
		"https://api.example.com/v1?key=SECRET",
		"https://api.example.com/v1?api_key=SECRET",
		"https://api.example.com/v1?API_KEY=SECRET",
		"https://api.example.com/v1?apiKey=SECRET",
		"https://api.example.com/v1?apikey=SECRET",
		"https://api.example.com/v1?access_token=SECRET",
		"https://api.example.com/v1?Token=SECRET",
		"https://api.example.com/v1?refresh-token=SECRET",
		"https://api.example.com/v1?client_secret=SECRET",
		"https://api.example.com/v1?secret=SECRET",
		"https://api.example.com/v1?signature=SECRET",
		"https://api.example.com/v1?sig=SECRET",
		"https://api.example.com/v1?password=SECRET",
		"https://api.example.com/v1?pwd=SECRET",
		"https://api.example.com/v1?auth=SECRET",
		"https://api.example.com/v1?X-Api-Key=SECRET",
		"https://api.example.com/v1?sessionId=SECRET",
		"https://api.example.com/v1?credentials=SECRET",
		// The worst of them all: the password in the userinfo, which url.String
		// prints whole.
		"https://user:SECRET@api.example.com/v1",
		// And combined, so it does not pass by accident on a single one.
		"https://user:SECRET@api.example.com/v1?token=SECRET&latitude=-23.5",
	}

	for _, c := range cases {
		got := redactURL(c)
		if strings.Contains(got, "SECRET") {
			t.Errorf("leaked:\n  %s\n  -> %s", c, got)
		}
	}
}

// Redacting too much must not erase what is useful for debugging: the URL has to
// stay recognizable.
func TestRedactPreservesWhatIsNotASecret(t *testing.T) {
	got := redactURL("https://api.open-meteo.com/v1/forecast?latitude=-23.55&longitude=-46.63&api_key=X")

	for _, wants := range []string{"api.open-meteo.com", "/v1/forecast", "latitude=-23.55", "longitude=-46.63"} {
		if !strings.Contains(got, wants) {
			t.Errorf("the URL lost %q, so it is no use for debugging: %s", wants, got)
		}
	}
	if !strings.Contains(got, "REDACTED") {
		t.Errorf("the key was not redacted: %s", got)
	}
}

// A user with no password is not a secret, and erasing it would remove useful
// information.
func TestRedactKeepsAUserWithNoPassword(t *testing.T) {
	got := redactURL("https://user@api.example.com/v1")
	if !strings.Contains(got, "user") {
		t.Errorf("the user without a password vanished: %s", got)
	}
}

func TestRedactDoesNotPanicOnAnInvalidURL(t *testing.T) {
	if got := redactURL("://this-is-not-a-url"); got != "[invalid url]" {
		t.Errorf("redactURL(invalid) = %q", got)
	}
}

// The redaction errs on the safe side: "monkey" contains "key" and becomes ***.
// It is the price of not depending on guessing the name the vendor chose.
func TestRedactErrsOnTheSafeSide(t *testing.T) {
	if got := redactURL("https://x/y?monkey=1"); !strings.Contains(got, "REDACTED") {
		t.Errorf("expected over-redaction: %s", got)
	}
}
