package sdk

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brevis.sh/brevis/sdk/from"
)

// fakeTarget is a Writer that returns a result and an error together, which is
// what every destination does on a refusal -- the result exists so RowErrors is
// readable afterwards.
type fakeTarget struct {
	failure bool
	rows    []string
}

func (fakeTarget) Describe() string { return "target.test" }

func (d fakeTarget) Write(context.Context, []Envelope, WriteOptions) (*LoadResult, error) {
	res := &LoadResult{RowsLoaded: 2, ErrorRows: d.rows}
	if d.failure {
		res.RowsLoaded = 0
		return res, context.DeadlineExceeded
	}
	return res, nil
}

func runCapturingLog(t *testing.T, target Writer) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	_ = runPipeline(context.Background(), &Pipeline{
		Source: Source{From: from.HTTP{URL: srv.URL}},
		Target: Target{To: target},
	})

	return buf.String()
}

// A load that did not load must not log "loaded".
//
// The result comes back filled on the error path by design, so RowErrors is
// readable -- which makes the message the only thing that tells the two cases
// apart. And a "loaded" at INFO during a failure never reaches whoever watches
// ERROR.
func TestTheLogDoesNotSayLoadedWhenItDidNot(t *testing.T) {
	output := runCapturingLog(t, fakeTarget{failure: true, rows: []string{"line 0 refused"}})

	if strings.Contains(output, "msg=loaded") {
		t.Errorf("a load that failed logged \"loaded\":\n%s", output)
	}
	if !strings.Contains(output, `msg="load failed"`) {
		t.Errorf("the failure has to show up in the summary line:\n%s", output)
	}
	if !strings.Contains(output, "level=ERROR msg=\"load failed\"") {
		t.Errorf("the line that summarizes a failure has to be ERROR:\n%s", output)
	}
	// And the counters are still there, which is why the result comes back.
	if !strings.Contains(output, "lines=0") || !strings.Contains(output, "records=2") {
		t.Errorf("the counters were lost in the swap:\n%s", output)
	}
	if !strings.Contains(output, "row rejected") {
		t.Errorf("the refused rows vanished:\n%s", output)
	}
}

func TestTheLogSaysLoadedWhenItLoaded(t *testing.T) {
	output := runCapturingLog(t, fakeTarget{})

	if !strings.Contains(output, "level=INFO msg=loaded") {
		t.Errorf("a load that worked has to log loaded at INFO:\n%s", output)
	}
	if strings.Contains(output, "load failed") {
		t.Errorf("a load that worked must not log a failure:\n%s", output)
	}
}
