package from_test

import (
	"testing"

	"brevis.sh/brevis/sdk/from"
)

// TestCampoJSONStillReadsWhatJSONTokenReads guards the deprecated alias.
//
// CampoJSON shipped in a published version. Deleting it would break a
// consumer's build with an error that names a symbol and explains nothing, so
// it stays until v1 -- and this test is what makes the deletion visible here
// instead of there.
func TestCampoJSONStillReadsWhatJSONTokenReads(t *testing.T) {
	body := []byte(`{"data":{"accessToken":"abc123"}}`)

	fresh, err := from.JSONToken("data.accessToken")(body)
	if err != nil {
		t.Fatalf("JSONToken: %v", err)
	}
	old, err := from.CampoJSON("data.accessToken")(body)
	if err != nil {
		t.Fatalf("CampoJSON: %v", err)
	}

	if fresh != old {
		t.Fatalf("the alias diverged: JSONToken=%q CampoJSON=%q", fresh, old)
	}
	if fresh != "abc123" {
		t.Fatalf("read %q, want %q", fresh, "abc123")
	}
}
