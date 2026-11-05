package from_test

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/sdk"
	"brevis.sh/brevis/sdk/from"
)

// fakeSource delivers N records, or fails.
type fakeSource struct {
	name     string
	lines    int
	openErr  error
	errAfter int // > 0: it fails after N rows
}

func (f fakeSource) Describe() string { return f.name }

func (f fakeSource) Read(context.Context, sdk.ReadOptions) (iter.Seq2[sdk.Envelope, error], error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return func(yield func(sdk.Envelope, error) bool) {
		for i := 0; i < f.lines; i++ {
			if f.errAfter > 0 && i == f.errAfter {
				yield(sdk.Envelope{}, fmt.Errorf("%s broke at line %d", f.name, i))
				return
			}
			if !yield(sdk.Envelope{Payload: map[string]any{"source": f.name, "i": i}}, nil) {
				return
			}
		}
	}, nil
}

func drain(t *testing.T, source sdk.Reader, stats *sdk.Stats) ([]map[string]any, error) {
	t.Helper()
	data, err := sdk.Extract(context.Background(), sdk.Source{From: source, Stats: stats})
	if err != nil {
		return nil, err
	}
	var lines []map[string]any
	for env, err := range data.Records {
		if err != nil {
			return lines, err
		}
		lines = append(lines, env.Payload.(map[string]any))
	}
	return lines, nil
}

// TestManyJoinsTheSources: the base case.
func TestManyJoinsTheSources(t *testing.T) {
	lines, err := drain(t, from.Many{Sources: []sdk.Reader{
		fakeSource{name: "a", lines: 2},
		fakeSource{name: "b", lines: 3},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Errorf("%d rows, want 5", len(lines))
	}
}

// TestManySequentialKeepsTheOrder: with Workers 0 or 1 the sequence is
// deterministic, and that is why it is the default. Concurrency is opt-in
// precisely because it gives that up.
func TestManySequentialKeepsTheOrder(t *testing.T) {
	sources := []sdk.Reader{
		fakeSource{name: "a", lines: 2},
		fakeSource{name: "b", lines: 2},
		fakeSource{name: "c", lines: 2},
	}
	var previous string
	for attempt := 0; attempt < 20; attempt++ {
		lines, err := drain(t, from.Many{Sources: sources}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		for _, l := range lines {
			order = append(order, fmt.Sprintf("%s%v", l["source"], l["i"]))
		}
		current := strings.Join(order, ",")
		if current != "a0,a1,b0,b1,c0,c1" {
			t.Fatalf("order = %s", current)
		}
		if attempt > 0 && current != previous {
			t.Fatalf("the order varied between runs: %s and %s", previous, current)
		}
		previous = current
	}
}

// TestManyAbortsByDefault: changing the default in silence would make a run that
// fails today start "working" with half the data.
func TestManyAbortsByDefault(t *testing.T) {
	_, err := drain(t, from.Many{Sources: []sdk.Reader{
		fakeSource{name: "boa", lines: 2},
		fakeSource{name: "bad", openErr: fmt.Errorf("504")},
		fakeSource{name: "other", lines: 2},
	}}, nil)
	if err == nil {
		t.Fatal("the default tolerated a failure")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("the error does not name the source: %v", err)
	}
}

// TestManyContinuesAndSaysWhichFailed is the whole of item 2 in one test.
//
// On a fan-out of thousands of sources, "the run failed" is not information:
// what fixes it is knowing WHICH failed, so those get reprocessed and the others
// do not.
func TestManyContinuesAndSaysWhichFailed(t *testing.T) {
	var stats sdk.Stats
	lines, err := drain(t, from.Many{
		Sources: []sdk.Reader{
			fakeSource{name: "a", lines: 2},
			fakeSource{name: "broken", openErr: fmt.Errorf("504 from the provider")},
			fakeSource{name: "c", lines: 3},
		},
		OnError: sdk.ContinueOnError,
	}, &stats)
	if err != nil {
		t.Fatalf("ContinueOnError aborted: %v", err)
	}
	if len(lines) != 5 {
		t.Errorf("%d rows, want 5 -- the good ones must survive the bad one", len(lines))
	}
	if len(stats.FailedSources) != 1 {
		t.Fatalf("failures = %v, want one", stats.FailedSources)
	}
	f := stats.FailedSources[0]
	if f.Source != "broken" || !strings.Contains(f.Err, "504") {
		t.Errorf("the failure does not say which or why: %+v", f)
	}
}

// TestAFailureMidwayThroughASourceCountsToo: a source that breaks on row 3
// failed just as much as one that never opened.
func TestAFailureMidwayThroughASourceCountsToo(t *testing.T) {
	var stats sdk.Stats
	lines, err := drain(t, from.Many{
		Sources: []sdk.Reader{
			fakeSource{name: "half", lines: 10, errAfter: 3},
			fakeSource{name: "whole", lines: 2},
		},
		OnError: sdk.ContinueOnError,
	}, &stats)
	if err != nil {
		t.Fatal(err)
	}
	// The 3 the source delivered before breaking count: they were read.
	if len(lines) != 5 {
		t.Errorf("%d rows, want 5 (3 from the half + 2 from the whole)", len(lines))
	}
	if len(stats.FailedSources) != 1 || stats.FailedSources[0].Source != "half" {
		t.Errorf("failures = %v", stats.FailedSources)
	}
}

// TestEverySourceFailingIsNotZeroRows: zero records from N good sources is a
// result; zero because all N failed is a broken run, and the two must not
// look like the same thing.
func TestEverySourceFailingIsNotZeroRows(t *testing.T) {
	_, err := drain(t, from.Many{
		Sources: []sdk.Reader{
			fakeSource{name: "a", openErr: fmt.Errorf("504")},
			fakeSource{name: "b", openErr: fmt.Errorf("503")},
		},
		OnError: sdk.ContinueOnError,
	}, nil)
	if err == nil {
		t.Fatal("all the sources failed and the run succeeded")
	}
	if !strings.Contains(err.Error(), "all 2 sources failed") {
		t.Errorf("the error does not say they all failed: %v", err)
	}
}

// TestManyConcurrentReadsEverything: with concurrency the order changes, the set
// does not.
func TestManyConcurrentReadsEverything(t *testing.T) {
	var sources []sdk.Reader
	for i := 0; i < 50; i++ {
		sources = append(sources, fakeSource{name: fmt.Sprintf("f%02d", i), lines: 4})
	}

	lines, err := drain(t, from.Many{Sources: sources, Workers: 8}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 200 {
		t.Fatalf("%d rows, want 200", len(lines))
	}
	seen := map[string]int{}
	for _, l := range lines {
		seen[l["source"].(string)]++
	}
	if len(seen) != 50 {
		t.Errorf("%d sources showed up, want 50", len(seen))
	}
	for name, n := range seen {
		if n != 4 {
			t.Errorf("%s delivered %d rows", name, n)
		}
	}
}

// TestManySumsTheCounters: the result's counters have to describe the whole
// read, and not the last source.
func TestManySumsTheCounters(t *testing.T) {
	var stats sdk.Stats
	if _, err := drain(t, from.Many{
		Sources: []sdk.Reader{counter{2}, counter{3}, counter{5}},
		Workers: 3,
	}, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Pages != 10 {
		t.Errorf("Pages = %d, want 10 (2+3+5)", stats.Pages)
	}
}

// countingSource fills the Stats it receives, like a real driver.
type counter struct{ pages int }

func (c counter) Describe() string { return fmt.Sprintf("counter(%d)", c.pages) }
func (c counter) Read(_ context.Context, opt sdk.ReadOptions) (iter.Seq2[sdk.Envelope, error], error) {
	return func(yield func(sdk.Envelope, error) bool) {
		if opt.Stats != nil {
			opt.Stats.Pages = c.pages
		}
		yield(sdk.Envelope{Payload: map[string]any{"n": c.pages}}, nil)
	}, nil
}

// TestManyStopsReadingWhenTheConsumerStops: a break in the consumer's loop must
// not leave a goroutine stuck writing into a channel nobody reads.
func TestManyStopsReadingWhenTheConsumerStops(t *testing.T) {
	var sources []sdk.Reader
	for i := 0; i < 20; i++ {
		sources = append(sources, fakeSource{name: fmt.Sprintf("f%d", i), lines: 1000})
	}

	data, err := sdk.Extract(context.Background(), sdk.Source{
		From: from.Many{Sources: sources, Workers: 4},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		n := 0
		for _, err := range data.Records {
			if err != nil {
				t.Error(err)
				return
			}
			n++
			if n == 10 {
				break
			}
		}
	}()

	select {
	case <-done:
	case <-timeout():
		t.Fatal("the iteration did not finish after the break; some goroutine got stuck")
	}
}

// TestManyRefusesInvalidConfiguration.
func TestManyRefusesInvalidConfiguration(t *testing.T) {
	if _, err := drain(t, from.Many{}, nil); err == nil {
		t.Error("accepted zero sources")
	}
	if _, err := drain(t, from.Many{Sources: []sdk.Reader{nil}}, nil); err == nil {
		t.Error("accepted a nil source")
	}
}

func timeout() <-chan struct{} {
	c := make(chan struct{})
	go func() { time.Sleep(10 * time.Second); close(c) }()
	return c
}

// TestDiscoverBuildsTheSourcesInsideThePipeline is the second half of item 9.
//
// The list is sometimes only known at run time. Built before sdk.Run, it sits
// outside the pipeline: no retry, no timeout, no log, and no appearance in the
// Result when it fails.
func TestDiscoverBuildsTheSourcesInsideThePipeline(t *testing.T) {
	lines, err := drain(t, from.Many{
		Discover: func(context.Context) ([]sdk.Reader, error) {
			return []sdk.Reader{
				fakeSource{name: "discovered-a", lines: 2},
				fakeSource{name: "discovered-b", lines: 3},
			}, nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Errorf("%d rows, want 5", len(lines))
	}
}

// TestAFailingDiscoverIsAnExtractError: its error is treated like
// any other extract error, and not as a panic in a main before anything
// starts.
func TestAFailingDiscoverIsAnExtractError(t *testing.T) {
	_, err := drain(t, from.Many{
		Discover: func(context.Context) ([]sdk.Reader, error) {
			return nil, fmt.Errorf("the endpoint that lists the partitions returned 503")
		},
	}, nil)
	if err == nil {
		t.Fatal("the discovery failed and the run carried on")
	}
	if !strings.Contains(err.Error(), "discovering the sources") {
		t.Errorf("the error does not say what failed: %v", err)
	}
}

// TestAnEmptyDiscoverIsNotZeroRecords: a run that read nothing because
// there was nothing to read is different from one that did not know where to
// read.
func TestAnEmptyDiscoverIsNotZeroRecords(t *testing.T) {
	_, err := drain(t, from.Many{
		Discover: func(context.Context) ([]sdk.Reader, error) { return nil, nil },
	}, nil)
	if err == nil {
		t.Fatal("zero sources passed as zero records")
	}
	if !strings.Contains(err.Error(), "returned no source at all") {
		t.Errorf("error = %v", err)
	}
}

// TestDiscoverAndSourcesTogetherIsRefused: two lists of sources, and the loser
// would lose in silence.
func TestDiscoverAndSourcesTogetherIsRefused(t *testing.T) {
	_, err := drain(t, from.Many{
		Sources:  []sdk.Reader{fakeSource{name: "a", lines: 1}},
		Discover: func(context.Context) ([]sdk.Reader, error) { return nil, nil },
	}, nil)
	if err == nil {
		t.Fatal("declaring both got through")
	}
}
