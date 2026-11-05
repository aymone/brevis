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

// paginatorWithMeta serves N pages and says, in the body, whether there is a
// next one.
func paginatorWithMeta(t *testing.T, pages int, withMeta bool) (*httptest.Server, *int) {
	t.Helper()
	var requested int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested++
		p := 1
		if v := r.URL.Query().Get("page"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &p)
		}
		hasMore := p < pages
		if withMeta {
			// It lies, always saying there is more; the safety net (an empty
			// page) has to end the run anyway.
			hasMore = true
		}
		lines := `[{"n":1}]`
		if p > pages {
			lines = `[]`
		}
		_, _ = fmt.Fprintf(w, `{"pageMeta":{"hasNextPage":%t},"results":%s}`, hasMore, lines)
	}))
	t.Cleanup(srv.Close)
	return srv, &requested
}

func count(t *testing.T, s core.Source) int {
	t.Helper()
	seq, err := JSON(context.Background(), s, nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	n := 0
	for _, err := range seq {
		if err != nil {
			t.Fatalf("iterating: %v", err)
		}
		n++
	}
	return n
}

// TestMoreKeyStopsWithoutAskingForTheEmptyPage is item 12: stopping on an empty
// page costs one extra request PER SOURCE, and on a fan-out of hundreds of
// sources that is hundreds of requests per run.
func TestMoreKeyStopsWithoutAskingForTheEmptyPage(t *testing.T) {
	srv, requested := paginatorWithMeta(t, 3, false)

	lines := count(t, core.Source{
		URL: srv.URL, PageKey: "page", DataKey: "results", MoreKey: "pageMeta.hasNextPage",
	})
	if lines != 3 {
		t.Errorf("%d rows, want 3", lines)
	}
	if *requested != 3 {
		t.Errorf("%d requests, want 3 -- the fourth is the one MoreKey saves", *requested)
	}
}

// TestWithoutMoreKeyItAsksForTheEmptyPage is the other side: without it, the
// fourth request happens. It is the measure of what the item saves.
func TestWithoutMoreKeyItAsksForTheEmptyPage(t *testing.T) {
	srv, requested := paginatorWithMeta(t, 3, false)

	count(t, core.Source{URL: srv.URL, PageKey: "page", DataKey: "results"})
	if *requested != 4 {
		t.Errorf("%d requests, want 4 -- the fourth comes back empty and is what ends it", *requested)
	}
}

// TestMoreKeyDoesNotReplaceTheSafetyNet: an API that lies in the field must not
// become an infinite loop.
func TestMoreKeyDoesNotReplaceTheSafetyNet(t *testing.T) {
	srv, requested := paginatorWithMeta(t, 2, true)

	lines := count(t, core.Source{
		URL: srv.URL, PageKey: "page", DataKey: "results", MoreKey: "pageMeta.hasNextPage",
	})
	if lines != 2 {
		t.Errorf("%d rows, want 2", lines)
	}
	// The third comes back empty and ends it, despite the field saying there is
	// more.
	if *requested != 3 {
		t.Errorf("%d requests; stopping on an empty page should have ended it", *requested)
	}
}

// TestAnAbsentMoreKeyIsNotTheEndOfPagination: treating absent as the end would
// make pagination stop at the first page in silence, which is worse than not
// having the optimization.
func TestAnAbsentMoreKeyIsNotTheEndOfPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"results":[{"n":1}]}`)
	}))
	defer srv.Close()

	// The error comes from the JSON and not from the iteration: the first page is
	// fetched early, on purpose, so a wrong path fails before the consumer drains
	// the sequence.
	_, err := JSON(context.Background(), core.Source{
		URL: srv.URL, DataKey: "results", MoreKey: "pageMeta.hasNextPage",
	}, nil)
	if err == nil {
		t.Fatal("a missing field passed as 'no more'")
	}
	for _, want := range []string{"pageMeta", "check the path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say %q: %v", want, err)
		}
	}
}

// TestMoreKeyAtTheRoot: a path with no dot works too -- has_more is common at
// the root.
func TestMoreKeyAtTheRoot(t *testing.T) {
	var requested int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested++
		p := 1
		if v := r.URL.Query().Get("page"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &p)
		}
		_, _ = fmt.Fprintf(w, `{"has_more":%t,"data":[{"n":%d}]}`, p < 2, p)
	}))
	defer srv.Close()

	if n := count(t, core.Source{
		URL: srv.URL, PageKey: "page", DataKey: "data", MoreKey: "has_more",
	}); n != 2 {
		t.Errorf("%d rows, want 2", n)
	}
	if requested != 2 {
		t.Errorf("%d requests, want 2", requested)
	}
}

// TestANullMoreKeyIsTheEnd: null is the form several APIs use for "that is
// all".
func TestANullMoreKeyIsTheEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"next":null,"data":[{"n":1}]}`)
	}))
	defer srv.Close()

	if n := count(t, core.Source{URL: srv.URL, DataKey: "data", MoreKey: "next"}); n != 1 {
		t.Errorf("%d rows, want 1", n)
	}
}

// TestAMoreKeyNotLeadingToABooleanIsAnError: a field that exists and is not a
// boolean is an error naming the type, and not a guess about its
// truthiness.
func TestAMoreKeyNotLeadingToABooleanIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"meta":{"next":"sim"},"data":[{"n":1}]}`)
	}))
	defer srv.Close()

	_, err := JSON(context.Background(), core.Source{
		URL: srv.URL, DataKey: "data", MoreKey: "meta.next",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Errorf("error = %v", err)
	}
}
