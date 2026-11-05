package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"brevis.sh/brevis/sdk/internal/core"
)

// fakeGCS is a pretend GCS with what matters here: a per-object generation,
// and a real
// ifGenerationMatch.
type fakeGCS struct {
	mu         sync.Mutex
	content    []byte
	generation int64
	exists     bool
	conflicts  int
}

func (g *fakeGCS) server(t *testing.T) *storage.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()

		// The client downloads the object with a GET on the XML path
		// (/bucket/object),
		// and not through the JSON API -- that is how the first version of this
		// fake
		// got it wrong.
		if r.Method == http.MethodGet {
			if !g.exists {
				http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
				return
			}
			w.Header().Set("x-goog-generation", fmt.Sprint(g.generation))
			w.Header().Set("Content-Length", fmt.Sprint(len(g.content)))
			_, _ = w.Write(g.content)
			return
		}

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/") {
			q := r.URL.Query()
			if v := q.Get("ifGenerationMatch"); v != "" {
				expected := v == fmt.Sprint(g.generation)
				if v == "0" {
					expected = !g.exists
				}
				if !expected {
					g.conflicts++
					http.Error(w, `{"error":{"code":412,"message":"generation mismatch"}}`,
						http.StatusPreconditionFailed)
					return
				}
			}
			body, _ := io.ReadAll(r.Body)
			g.content = extractMultipartBody(body)
			g.generation++
			g.exists = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "obj", "bucket": "b", "generation": fmt.Sprint(g.generation),
			})
			return
		}
		http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	cli, err := storage.NewClient(context.Background(),
		option.WithEndpoint(srv.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// extractMultipartBody takes the second part of the GCS multipart upload.
func extractMultipartBody(b []byte) []byte {
	parts := strings.Split(string(b), "\r\n\r\n")
	if len(parts) < 3 {
		return b
	}
	body := parts[2]
	if i := strings.LastIndex(body, "\r\n--"); i >= 0 {
		body = body[:i]
	}
	return []byte(body)
}

func credential(t *testing.T, g *fakeGCS) Credential {
	t.Helper()
	t.Setenv(core.EnvCredentialKey, "")
	generations.Delete("gs://b/obj")
	return Credential{Bucket: "b", Object: "obj", Client: g.server(t)}
}

// TestItStoresAndReturns: the happy path.
func TestItStoresAndReturns(t *testing.T) {
	g := &fakeGCS{}
	c := credential(t, g)

	if v, err := c.Load(); err != nil || v != "" {
		t.Fatalf("missing object = (%q, %v); should be empty with no error", v, err)
	}
	if err := c.Save("session=abc=="); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := c.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "session=abc==" {
		t.Errorf("Load = %q", got)
	}
}

// TestTheConditionalWrite: whoever read generation 1 and tries to write after
// somebody else wrote 2 gets a 412 -- and does NOT overwrite. That is the
// difference between a real CAS and last-writer-wins, which is what a volume
// would allow.
func TestTheConditionalWrite(t *testing.T) {
	g := &fakeGCS{}
	c := credential(t, g)

	if err := c.Save("first"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(); err != nil { // remembers generation 1
		t.Fatal(err)
	}

	// Another process writes from outside, advancing the generation.
	g.mu.Lock()
	g.generation++
	g.content = []byte("brevis-cred/1p\nfrom-another-process")
	g.mu.Unlock()

	// This run's write was conditional on the generation it read.
	if err := c.Save("my-value"); err != nil {
		t.Fatalf("the conflict became an error: %v", err)
	}
	g.mu.Lock()
	conflicts, content := g.conflicts, string(g.content)
	g.mu.Unlock()

	if conflicts != 1 {
		t.Errorf("conflicts = %d, want 1 -- the write was not conditional", conflicts)
	}
	if !strings.Contains(content, "from-another-process") {
		t.Errorf("overwrote the newer value: %q", content)
	}
}

// TestTheFirstWriteUsesDoesNotExist: without it, two first runs
// simultaneous ones would both write, and the older could arrive last.
func TestTheFirstWriteUsesDoesNotExist(t *testing.T) {
	g := &fakeGCS{}
	c := credential(t, g)

	if _, err := c.Load(); err != nil { // missing object, generation 0
		t.Fatal(err)
	}
	// Another process creates the object first.
	g.mu.Lock()
	g.exists, g.generation, g.content = true, 7, []byte("brevis-cred/1p\nfrom-another")
	g.mu.Unlock()

	if err := c.Save("my"); err != nil {
		t.Fatalf("the conflict became an error: %v", err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conflicts != 1 {
		t.Errorf("conflicts = %d; the first write did not use DoesNotExist", g.conflicts)
	}
	if !strings.Contains(string(g.content), "from-another") {
		t.Errorf("overwrote: %q", g.content)
	}
}

// TestBucketAndObjectAreRequired.
func TestBucketAndObjectAreRequired(t *testing.T) {
	for _, c := range []Credential{{Object: "o"}, {Bucket: "b"}, {}} {
		if err := c.CheckStore(); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}

// TestDescribeRevealsNothing: Describe reaches the log.
func TestDescribeRevealsNothing(t *testing.T) {
	c := Credential{Bucket: "b", Object: "o", Key: "secret-key-here"}
	if strings.Contains(c.Describe(), "secret-key") {
		t.Errorf("Describe leaks the key: %q", c.Describe())
	}
	if c.Describe() != "gs://b/o" {
		t.Errorf("Describe = %q", c.Describe())
	}
}
