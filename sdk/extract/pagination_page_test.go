package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk/internal/core"
)

// numberedPage returns three one-row pages and then an empty one, recording the
// sequence of numbers it received.
func numberedPage(t *testing.T, key string, seenValues *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get(key)
		*seenValues = append(*seenValues, raw)

		n, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "sem numero de pagina: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		if len(*seenValues) > 6 { // trava contra loop infinito no teste
			http.Error(w, "paginou demais", http.StatusInternalServerError)
			return
		}
		if n >= 3 {
			_, _ = fmt.Fprint(w, `{"results":[]}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"results":[{"n":%d}]}`, n)
	}))
}

// TestPageKeyAdvancesOneAtATime: the reason the field exists. Before it the
// recipe
// was OffsetKey "page" with PageSize 1, which worked by accident.
func TestPageKeyAdvancesOneAtATime(t *testing.T) {
	var seenValues []string
	srv := numberedPage(t, "page", &seenValues)
	defer srv.Close()

	lines := gather(t, core.Source{URL: srv.URL, PageKey: "page", DataKey: "results"})

	if lines != 2 {
		t.Errorf("rows = %d, want 2 (pages 1 and 2)", lines)
	}
	if got := strings.Join(seenValues, ","); got != "1,2,3" {
		t.Errorf("pages requested = %q, want \"1,2,3\"", got)
	}
}

// TestPageKeyNumbersTheFirstRequest: without it the server picks its own default
// and the SDK guesses the next number -- guessing wrong skips a page
// inteira em silencio.
func TestPageKeyNumbersTheFirstRequest(t *testing.T) {
	cases := []struct {
		name  string
		url   func(string) string
		first int
		seq   string
	}{
		{"padrao comeca em 1", func(u string) string { return u }, 0, "1,2,3"},
		{"FirstPage escolhe onde comecar", func(u string) string { return u }, 2, "2,3"},
		{"numero na propria url vence", func(u string) string { return u + "?page=2" }, 0, "2,3"},
		{"a url vence ate contra FirstPage", func(u string) string { return u + "?page=2" }, 9, "2,3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seenValues []string
			srv := numberedPage(t, "page", &seenValues)
			defer srv.Close()

			gather(t, core.Source{
				URL:       c.url(srv.URL),
				PageKey:   "page",
				FirstPage: c.first,
				DataKey:   "results",
			})
			if got := strings.Join(seenValues, ","); got != c.seq {
				t.Errorf("pages requested = %q, want %q", got, c.seq)
			}
		})
	}
}

// TestAZeroIndexedAPISaysSoInTheURL: FirstPage cannot express "start at zero",
// because zero is the value of somebody who set nothing. The documented way out
// is putting page zero in the URL. This test exists so the doc stays true.
func TestAZeroIndexedAPISaysSoInTheURL(t *testing.T) {
	var seenValues []string
	srv := numberedPage(t, "page", &seenValues)
	defer srv.Close()

	gather(t, core.Source{URL: srv.URL + "?page=0", PageKey: "page", DataKey: "results"})
	if got := strings.Join(seenValues, ","); got != "0,1,2,3" {
		t.Errorf("pages requested = %q, want \"0,1,2,3\"", got)
	}
}

// TestPaginationRefusesTwoStrategies: with two set, one would be read and the
// other ignored in silence -- which is the defect this SDK keeps finding.
func TestPaginationRefusesTwoStrategies(t *testing.T) {
	cases := []core.Source{
		{URL: "http://x", PageKey: "page", OffsetKey: "offset"},
		{URL: "http://x", CursorKey: "c", PageKey: "page"},
		{URL: "http://x", FollowLinks: true, OffsetKey: "offset"},
	}
	for _, s := range cases {
		if _, err := JSON(context.Background(), s, nil); err == nil {
			t.Errorf("accepted two strategies: %+v", s)
		}
	}
}

// TestPageSizeAloneIsRefused: PageSize is OffsetKey's step. Set without it, it
// does nothing -- and whoever wrote it believed it did.
func TestPageSizeAloneIsRefused(t *testing.T) {
	_, err := JSON(context.Background(), core.Source{URL: "http://x", PageSize: 100}, nil)
	if err == nil {
		t.Fatal("PageSize without OffsetKey got through")
	}
	if !strings.Contains(err.Error(), "PageKey") {
		t.Errorf("the error does not point to PageKey: %v", err)
	}

	_, err = JSON(context.Background(), core.Source{URL: "http://x", FirstPage: 2}, nil)
	if err == nil {
		t.Fatal("FirstPage without PageKey got through")
	}
}

func gather(t *testing.T, s core.Source) int {
	t.Helper()
	seq, err := JSON(context.Background(), s, nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	n := 0
	for _, err := range seq {
		if err != nil {
			t.Fatalf("iterando: %v", err)
		}
		n++
	}
	return n
}
