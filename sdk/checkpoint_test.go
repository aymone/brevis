package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countedSource counts how many times it was read and can refuse the second
// read.
//
// Refusing is what makes the test able to fail: a checkpoint that did not spare
// the source would go unnoticed if the source simply answered again.
type countedSource struct {
	records  []any
	reads    *int
	onlyOnce bool
}

func (countedSource) Describe() string { return "origem.teste" }

func (o countedSource) Read(context.Context, ReadOptions) (iter.Seq2[Envelope, error], error) {
	*o.reads++
	if o.onlyOnce && *o.reads > 1 {
		return nil, fmt.Errorf("the source was queried %d times", *o.reads)
	}
	regs := o.records
	return func(yield func(Envelope, error) bool) {
		for _, r := range regs {
			if !yield(Envelope{Payload: r}, nil) {
				return
			}
		}
	}, nil
}

// keepingTarget stores what it received, including when it refuses the load.
type keepingTarget struct {
	received *[]Envelope
	fail     bool
}

func (keepingTarget) Describe() string { return "destino.teste" }

func (d keepingTarget) Write(_ context.Context, envs []Envelope, _ WriteOptions) (*LoadResult, error) {
	*d.received = append(*d.received, envs...)
	if d.fail {
		return &LoadResult{}, fmt.Errorf("o destino recusou a carga")
	}
	return &LoadResult{RowsLoaded: int64(len(envs)), Strategy: "teste"}, nil
}

func runIt(t *testing.T, p *Pipeline) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	err := runPipeline(context.Background(), p)
	return buf.String(), err
}

func twoRecords() []any {
	return []any{
		map[string]any{"provider": "p", "entity": "e", "source_key": "a", "record_ts": "2026-09-05T00:00:00Z"},
		map[string]any{"provider": "p", "entity": "e", "source_key": "b", "record_ts": "2026-09-05T01:00:00Z"},
	}
}

// PHASE 1 -- the second attempt must not touch the source.
//
// It is the whole request: the extract spent the vendor's quota, the
// destination
// refused, and the next attempt has to load the same data without going back.
func TestOnTheSecondAttemptTheCheckpointDoesNotTouchTheSource(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads, onlyOnce: true}

	var first []Envelope
	_, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: src},
		Checkpoint: Checkpoint{At: dir},
		Target:     Target{To: keepingTarget{received: &first, fail: true}},
		Run:        RunContext{ID: "run-1", Attempt: 0},
	})
	if err == nil {
		t.Fatal("the first attempt should have failed at the destination")
	}

	var second []Envelope
	log, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: src},
		Checkpoint: Checkpoint{At: dir},
		Target:     Target{To: keepingTarget{received: &second}},
		Run:        RunContext{ID: "run-1", Attempt: 1},
	})
	if err != nil {
		t.Fatalf("the second attempt failed: %v\n%s", err, log)
	}

	if reads != 1 {
		t.Errorf("the source was read %d times; the checkpoint saved nothing", reads)
	}
	if len(second) != 2 {
		t.Fatalf("the resume loaded %d records, want 2", len(second))
	}
	if !strings.Contains(log, "checkpoint=reused") {
		t.Errorf("the log does not say it reused the checkpoint; an invisible saving is indistinguishable from no saving:\n%s", log)
	}
}

// A depot written by an SDK before the names went English (`_completo`,
// `parte-NNNNN.ndjson` and Portuguese manifest keys) still resumes: reading it
// has to spend none of the vendor's quota again.
func TestADepotWrittenBeforeTheRenameStillResumes(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads}

	var box []Envelope
	p := func(attempt int) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Target:     Target{To: keepingTarget{received: &box}},
			Run:        RunContext{ID: "run-legacy", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}

	// Turn the depot into what the earlier SDK wrote.
	part := find(t, dir, "part-00000.ndjson")
	legacyPart := filepath.Join(filepath.Dir(part), "parte-00000.ndjson")
	if err := os.Rename(part, legacyPart); err != nil {
		t.Fatal(err)
	}
	manifest := find(t, dir, "_complete")
	var m map[string]any
	readJSON(t, manifest, &m)
	writeJSON(t, filepath.Join(filepath.Dir(manifest), "_completo"), map[string]any{
		"versao": m["version"], "registros": m["records"], "partes": []string{"parte-00000.ndjson"},
		"numeros": m["numbers"], "pipeline": m["pipeline"], "run": m["run"], "gravado_em": m["written_at"],
	})
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}

	log, err := runIt(t, p(1))
	if err != nil {
		t.Fatalf("second attempt: %v\n%s", err, log)
	}
	if reads != 1 {
		t.Errorf("the source was read %d times; an old depot has to resume", reads)
	}
	if !strings.Contains(log, "checkpoint=reused") {
		t.Errorf("the log does not say it reused the checkpoint:\n%s", log)
	}
}

// PHASE 2 (I1) -- with no manifest there is no resume.
func TestACheckpointWithNoManifestRedoesTheExtract(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads}

	var box []Envelope
	p := func(attempt int) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Target:     Target{To: keepingTarget{received: &box}},
			Run:        RunContext{ID: "run-2", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}

	// The manifest is what authorizes the resume. Without it the depot is an
	// extract
	// interrupted, and resuming from there would load half the data in
	// silence.
	remove(t, dir, "_complete")

	log, err := runIt(t, p(1))
	if err != nil {
		t.Fatalf("second attempt: %v\n%s", err, log)
	}
	if reads != 2 {
		t.Errorf("the source was read %d times; without a manifest the extract has to be redone", reads)
	}
}

// A whole manifest with a part missing: refuse before loading anything.
func TestACheckpointWithAMissingPartRedoesTheExtract(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads}

	var box []Envelope
	p := func(attempt int) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Target:     Target{To: keepingTarget{received: &box}},
			Run:        RunContext{ID: "run-3", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	remove(t, dir, "part-00000.ndjson")

	log, err := runIt(t, p(1))
	if err != nil {
		t.Fatalf("second attempt: %v\n%s", err, log)
	}
	if reads != 2 {
		t.Errorf("the source was read %d times; a lame depot has to be discarded", reads)
	}
	if !strings.Contains(log, "checkpoint incomplete") {
		t.Errorf("discarding a checkpoint silently hides the cause:\n%s", log)
	}
	// And what was loaded has to be the whole set, not what was left over.
	if len(box) != 4 {
		t.Errorf("loaded %d records across the two attempts, want 4", len(box))
	}
}

// PHASE 2 -- a manifest that lies about the count fails LOUDLY.
//
// This can only happen with the object tampered with after it was written.
// Carrying on quietly would load fewer rows than the first attempt loaded,
// and
// ninguem saberia.
func TestACheckpointWithAWrongCountFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads}

	var box []Envelope
	p := func(attempt int) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Target:     Target{To: keepingTarget{received: &box}},
			Run:        RunContext{ID: "run-4", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}

	path := find(t, dir, "_complete")
	var m map[string]any
	readJSON(t, path, &m)
	m["records"] = 99
	writeJSON(t, path, m)

	_, err := runIt(t, p(1))
	if err == nil {
		t.Fatal("a checkpoint that lies about its count has to fail, not silently load less")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("the message does not say what happened: %v", err)
	}
}

// PHASE 3 (I3) -- a resumed attempt's ingestion_ids are identical.
//
// It is the guarantee the request calls "loading the same data".
func TestTheCheckpointPreservesTheIngestionID(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{records: twoRecords(), reads: &reads, onlyOnce: true}

	var fromSource, doCheckpoint []Envelope
	p := func(attempt int, box *[]Envelope) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Transform:  []Transformer{IngestionID()},
			Target:     Target{To: keepingTarget{received: box}},
			Run:        RunContext{ID: "run-5", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0, &fromSource)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if log, err := runIt(t, p(1, &doCheckpoint)); err != nil {
		t.Fatalf("retomada: %v\n%s", err, log)
	}

	if len(fromSource) != len(doCheckpoint) {
		t.Fatalf("%d records from the source against %d from the checkpoint", len(fromSource), len(doCheckpoint))
	}
	for i := range fromSource {
		a := fromSource[i].Payload.(map[string]any)["ingestion_id"]
		b := doCheckpoint[i].Payload.(map[string]any)["ingestion_id"]
		if a != b {
			t.Errorf("record %d: id %v from the source, %v from the checkpoint", i, a, b)
		}
		if a == nil || a == "" {
			t.Fatalf("record %d has no ingestion_id; the test is not comparing anything", i)
		}
	}
}

// PHASE 3 -- the number's literal survives the round trip through NDJSON.
//
// A payload with a json.Number carries `19.0`; read back as a float64 it would
// become
// "19" in asText, and the resume would write an ingestion_id different from
// first attempt. The mode is recorded in the manifest.
func TestTheCheckpointPreservesTheNumbersLiteral(t *testing.T) {
	dir := t.TempDir()
	var reads int
	src := countedSource{
		records: []any{map[string]any{"id": json.Number("19.0"), "n": json.Number("1e21")}},
		reads:   &reads, onlyOnce: true,
	}

	var box []Envelope
	p := func(attempt int) *Pipeline {
		return &Pipeline{
			Name: "fetcher", Source: Source{From: src},
			Checkpoint: Checkpoint{At: dir},
			Target:     Target{To: keepingTarget{received: &box}},
			Run:        RunContext{ID: "run-6", Attempt: attempt},
		}
	}
	if _, err := runIt(t, p(0)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if log, err := runIt(t, p(1)); err != nil {
		t.Fatalf("retomada: %v\n%s", err, log)
	}

	if len(box) != 2 {
		t.Fatalf("loaded %d records, want 2", len(box))
	}
	for i, env := range box {
		row := env.Payload.(map[string]any)
		if got := asText(row["id"]); got != "19.0" {
			t.Errorf("record %d: id became %q, want \"19.0\"", i, got)
		}
		if got := asText(row["n"]); got != "1e21" {
			t.Errorf("record %d: n became %q, want \"1e21\"", i, got)
		}
	}
}

// PHASE 4 (I5) -- a depot that cannot write warns, and the run goes on.
//
// The checkpoint is an insurance policy, not the product. Dying because of the
// insurance would be
// trading a rare failure for a failure on every run.
func TestACheckpointThatCannotWriteDoesNotFailTheRun(t *testing.T) {
	var reads int
	var box []Envelope
	log, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: countedSource{records: twoRecords(), reads: &reads}},
		Checkpoint: Checkpoint{At: "s3://balde/cp", Store: refusingStore{}},
		Target:     Target{To: keepingTarget{received: &box}},
		Run:        RunContext{ID: "run-7", Attempt: 0},
	})
	if err != nil {
		t.Fatalf("the run died because of the checkpoint: %v\n%s", err, log)
	}
	if len(box) != 2 {
		t.Errorf("loaded %d records, want 2", len(box))
	}
	if !strings.Contains(log, "checkpoint_failed") {
		t.Errorf("the depot's failure has to show up in the result:\n%s", log)
	}
}

// PHASE 4 (I5) -- failing MIDWAY does not fail the run either, and does not
// re-read the source.
//
// Here `_start` writes and the parts do not. The stream degrades: it yields
// what already became
// object, what stayed in the buffer, and carries on straight from the source --
// without redoing the extract, which is precisely what was being spared.
func TestACheckpointFailingMidwayDegradesWithoutRereadingTheSource(t *testing.T) {
	var reads int
	var box []Envelope
	log, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: countedSource{records: twoRecords(), reads: &reads, onlyOnce: true}},
		Checkpoint: Checkpoint{At: "s3://balde/cp", Store: storeRefusingParts{}},
		Target:     Target{To: keepingTarget{received: &box}},
		Run:        RunContext{ID: "run-8", Attempt: 0},
	})
	if err != nil {
		t.Fatalf("the run died because of the checkpoint: %v\n%s", err, log)
	}
	if reads != 1 {
		t.Errorf("the source was read %d times; degrading must not cost a second extraction", reads)
	}
	if len(box) != 2 {
		t.Errorf("loaded %d records, want 2 -- degrading must not lose a row", len(box))
	}
	if !strings.Contains(log, "checkpoint_failed") {
		t.Errorf("the interruption has to show up in the result:\n%s", log)
	}
}

// Outside the engine there is no stable key, and that is SAID.
func TestOutsideTheEngineTheCheckpointWarnsInsteadOfIgnoring(t *testing.T) {
	var reads int
	var box []Envelope
	log, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: countedSource{records: twoRecords(), reads: &reads}},
		Checkpoint: Checkpoint{At: t.TempDir()},
		Target:     Target{To: keepingTarget{received: &box}},
		Run:        RunContext{}, // no id: running by hand
	})
	if err != nil {
		t.Fatalf("running by hand must not fail: %v", err)
	}
	if !strings.Contains(log, "checkpoint off") {
		t.Errorf("switching off silently hides a setting that is not in effect:\n%s", log)
	}
}

// A scheme that does not match the Store is an ERROR, not a warning: it will
// never write.
func TestACheckpointWithTheWrongStoreIsAnError(t *testing.T) {
	var reads int
	var box []Envelope
	_, err := runIt(t, &Pipeline{
		Name:       "fetcher",
		Source:     Source{From: countedSource{records: twoRecords(), reads: &reads}},
		Checkpoint: Checkpoint{At: "gs://balde/cp", Store: refusingStore{}}, // store e s3
		Target:     Target{To: keepingTarget{received: &box}},
		Run:        RunContext{ID: "run-9"},
	})
	if err == nil {
		t.Fatal("a Store that will never serve the path has to be refused immediately")
	}
	if !strings.Contains(err.Error(), "gs") || !strings.Contains(err.Error(), "s3") {
		t.Errorf("the message has to name both sides: %v", err)
	}
	if reads != 0 {
		t.Error("refused after spending the source")
	}
}

// --- stores de teste ---

type refusingStore struct{}

func (refusingStore) Scheme() string { return "s3" }
func (refusingStore) List(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (refusingStore) Open(context.Context, string, string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("sem permissao de leitura")
}
func (refusingStore) Create(context.Context, string, string, io.Reader) error {
	return fmt.Errorf("sem permissao de escrita")
}

// storeRefusingParts allows the reservation and refuses the parts: it is the
// failure that shows up mid-extraction, after the quota has already been
// spent.
type storeRefusingParts struct{ refusingStore }

func (storeRefusingParts) Create(_ context.Context, _, key string, _ io.Reader) error {
	if strings.Contains(key, "part-") {
		return fmt.Errorf("balde cheio")
	}
	return nil
}

// --- file helpers ---

func find(t *testing.T, root, name string) string {
	t.Helper()
	var found string
	err := filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err == nil && filepath.Base(p) == name {
			found = p
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("could not find %q in %s", name, root)
	}
	return found
}

func remove(t *testing.T, root, name string) {
	t.Helper()
	if err := os.Remove(find(t, root, name)); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // caminho de teste
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
