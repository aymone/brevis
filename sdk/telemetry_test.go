package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"strings"
	"testing"
	"time"
)

// slowSource delivers records with a pause between them, so the extraction's
// duration is measurable rather than zero.
type slowSource struct {
	records []any
	pause   time.Duration
}

func (slowSource) Describe() string { return "source.slow" }

func (o slowSource) Read(context.Context, ReadOptions) (iter.Seq2[Envelope, error], error) {
	return func(yield func(Envelope, error) bool) {
		for _, r := range o.records {
			time.Sleep(o.pause)
			if !yield(Envelope{Payload: r}, nil) {
				return
			}
		}
	}, nil
}

// stagesOf runs the pipeline and returns the marked lines it announced.
func stagesOf(t *testing.T, p *Pipeline) ([]map[string]any, error) {
	t.Helper()
	var buf bytes.Buffer
	previous := phaseOutput
	phaseOutput = &buf
	defer func() { phaseOutput = previous }()

	err := runPipeline(context.Background(), p)

	var events []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, phaseMarker) {
			continue
		}
		var ev map[string]any
		if e := json.Unmarshal([]byte(strings.TrimPrefix(line, phaseMarker)), &ev); e != nil {
			t.Fatalf("line marked unreadable %q: %v", line, e)
		}
		events = append(events, ev)
	}
	return events, err
}

func testPipeline(src Reader, box *[]Envelope) *Pipeline {
	return &Pipeline{
		Name:      "fetcher",
		Source:    Source{From: src},
		Transform: []Transformer{SkipWithout("id")},
		Target:    Target{To: keepingTarget{received: box}},
		Run:       RunContext{ID: "run-stages", Attempt: 0},
	}
}

func TestPhasesFollowThePipelineShape(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, testPipeline(countedSource{
		records: []any{map[string]any{"id": 1}, map[string]any{"id": 2}},
		reads:   new(int),
	}, &box))
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	var trail []string
	for _, ev := range events {
		if ev["type"] == "sdk" || ev["type"] == "pipeline" {
			trail = append(trail, ev["type"].(string))
			continue
		}
		trail = append(trail, ev["name"].(string)+":"+ev["state"].(string))
	}
	// One phase per element of the pipeline's SHAPE: the source, each stage in
	// order, the destination. A single `transform` box could not tell "216
	// became 9" from "216 became 216 and then 9".
	wantedList := []string{
		"sdk", "pipeline",
		"check:running", "check:done",
		"extract:running",
		"map:running",
		"extract:done", "map:done",
		"load:running", "load:done",
	}
	if strings.Join(trail, " ") != strings.Join(wantedList, " ") {
		t.Errorf("trail:\n  %v\nexpected:\n  %v", trail, wantedList)
	}
}

// The announcement carries the version, because a badge that only says "SDK" is
// true and useless: the version is what answers "why does this step behave
// differently from its neighbor" without anybody opening the Dockerfile.
func TestTheAnnouncementCarriesTheVersion(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, testPipeline(countedSource{
		records: []any{map[string]any{"id": 1}}, reads: new(int),
	}, &box))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0]["type"] != "sdk" {
		t.Fatalf("the first event has to be the announcement, got %v", events)
	}
	if v, _ := events[0]["version"].(string); v == "" {
		t.Error("the announcement came out without a version")
	}
	if events[0]["pipeline"] != "fetcher" {
		t.Errorf("the announcement does not say whose it is: %v", events[0])
	}
}

// The extraction ends when the STREAM is exhausted, not when Extract returns
// the iterator.
//
// The chain is lazy: timing the calls would say "extract: 3ms" on a forty-minute
// extraction, and the screen would lie about precisely the longest stage.
func TestTheSourceDurationMeasuresTheRealExtraction(t *testing.T) {
	var box []Envelope
	p := testPipeline(slowSource{
		records: []any{map[string]any{"id": 1}, map[string]any{"id": 2}, map[string]any{"id": 3}},
		pause:   20 * time.Millisecond,
	}, &box)
	events, err := stagesOf(t, p)
	if err != nil {
		t.Fatal(err)
	}

	ms := stageDuration(t, events, "extract")
	if ms < 50 {
		t.Errorf("extract took %vms; the source spends 60ms, so the measurement is "+
			"timing the call instead of the stream", ms)
	}
}

// The transform reports no duration. It runs per record, interleaved with the
// read, so any number coming out of there would be the time of something
// else.
func TestAMapStageInventsNoDuration(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, testPipeline(slowSource{
		records: []any{map[string]any{"id": 1}}, pause: 20 * time.Millisecond,
	}, &box))
	if err != nil {
		t.Fatal(err)
	}
	end := findStage(t, events, "map", "done")
	if _, has := end["ms"]; has {
		t.Errorf("a map stage reported a duration: %v", end)
	}
}

// What only the transform knows: how many went in, how many came out, how many
// were
// dropped.
func TestAMapStageSaysHowManyItDropped(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, testPipeline(countedSource{
		records: []any{
			map[string]any{"id": 1},
			map[string]any{"without_id": true},
			map[string]any{"id": 3},
		},
		reads: new(int),
	}, &box))
	if err != nil {
		t.Fatal(err)
	}
	end := findStage(t, events, "map", "done")
	if end["in"] != 3.0 || end["out"] != 2.0 || end["dropped"] != 1.0 {
		t.Errorf("wrong count: %v", end)
	}
}

// Outside the engine there is nobody to read the stages, and cluttering the
// terminal of whoever is debugging a fetcher would be cost with no return.
func TestOutsideTheEngineNothingIsAnnounced(t *testing.T) {
	var box []Envelope
	p := testPipeline(countedSource{
		records: []any{map[string]any{"id": 1}}, reads: new(int),
	}, &box)
	p.Run = RunContext{} // running by hand

	events, err := stagesOf(t, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("announced %d events when run by hand: %v", len(events), events)
	}
	// And the pipeline keeps working the same.
	if len(box) != 1 {
		t.Errorf("loaded %d records, want 1", len(box))
	}
}

// The ceiling exists because the log stream becomes a database write on the
// other side: a pipeline in a loop would take Postgres down through the log's
// path.
func TestThePhaseCap(t *testing.T) {
	var buf bytes.Buffer
	previous := phaseOutput
	phaseOutput = &buf
	defer func() { phaseOutput = previous }()

	r := newReporter(RunContext{ID: "run-1"})
	for i := 0; i < phaseCap*3; i++ {
		r.started(PhaseSource)
	}
	if n := strings.Count(buf.String(), phaseMarker); n != phaseCap {
		t.Errorf("emitted %d rows, the ceiling is %d", n, phaseCap)
	}
}

func findStage(t *testing.T, events []map[string]any, name, state string) map[string]any {
	t.Helper()
	for _, ev := range events {
		if ev["name"] == name && ev["state"] == state {
			return ev
		}
	}
	t.Fatalf("could not find the stage %s:%s in %v", name, state, events)
	return nil
}

func stageDuration(t *testing.T, events []map[string]any, name string) float64 {
	t.Helper()
	ms, ok := findStage(t, events, name, "done")["ms"].(float64)
	if !ok {
		t.Fatalf("the stage %s did not report a duration", name)
	}
	return ms
}

// The screen has to show the pipeline the consumer DECLARED.
//
// Two Map stages share a name, and the phases used to be keyed by name -- so
// three declared stages collapsed into two boxes, and a single `transform` box
// could not tell "216 rows became 9" from "216 became 216 and then 9". The
// structure never reached the display.
func TestEachStageGetsItsOwnBox(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, &Pipeline{
		Name:   "fetcher",
		Source: Source{From: countedSource{records: twoRecords(), reads: new(int)}},
		Stages: []Stage{
			Map(SkipWithout("provider")),
			Aggregate(Reduce{By: GroupBy("provider"), Agg: map[string]Aggregator{"n": Count()}}),
			Map(Compute("x", func(map[string]any) (any, error) { return 1, nil })),
		},
		Target: Target{To: keepingTarget{received: &box}},
		Run:    RunContext{ID: "run-boxes"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Every phase that finished, by position.
	byIndex := map[float64]map[string]any{}
	for _, ev := range events {
		if ev["type"] == "stage" && ev["state"] == "done" {
			byIndex[ev["index"].(float64)] = ev
		}
	}
	if len(byIndex) != 6 {
		t.Fatalf("%d boxes finished, want 6 (check, source, map, aggregate, map, target): %v",
			len(byIndex), byIndex)
	}

	names := []string{"check", "extract", "map", "aggregate", "map", "load"}
	for i, want := range names {
		ev := byIndex[float64(i)]
		if ev == nil {
			t.Fatalf("no box at position %d", i)
		}
		if ev["name"] != want {
			t.Errorf("position %d is %q, want %q", i, ev["name"], want)
		}
	}

	// The aggregation's numbers belong to the aggregation, not to a sum of
	// everything: 2 records in, 1 group out.
	agg := byIndex[3]
	if agg["in"] != 2.0 || agg["out"] != 1.0 || agg["groups"] != 1.0 {
		t.Errorf("the aggregation's counts are wrong: %v", agg)
	}
	// And the map after it saw the aggregated row, not the raw ones.
	if after := byIndex[4]; after["in"] != 1.0 {
		t.Errorf("the map after the aggregation saw %v records, want 1", after["in"])
	}
}

// The card is half a card without saying WHICH source and WHICH destination.
func TestTheSourceAndTargetSayWhichTheyAre(t *testing.T) {
	var box []Envelope
	events, err := stagesOf(t, testPipeline(countedSource{
		records: twoRecords(), reads: new(int),
	}, &box))
	if err != nil {
		t.Fatal(err)
	}
	if d := findStage(t, events, "extract", "done")["detail"]; d != "source.test" {
		t.Errorf("the source does not say which it is: %v", d)
	}
	if d := findStage(t, events, "load", "done")["detail"]; d != "target.test" {
		t.Errorf("the destination does not say which it is: %v", d)
	}
}
