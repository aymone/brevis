package to

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "brevis.sh/brevis/sdk/internal/core"
)

func lote(n int) []core.Envelope {
	out := make([]core.Envelope, n)
	for i := range out {
		out[i] = core.Envelope{
			Provider: "acme", Entity: "widgets",
			SourceKey: string(rune('a' + i)), RecordTS: "2026-01-01T00:00:00Z",
			Payload: map[string]any{"sku": string(rune('A' + i)), "quantidade": i},
		}
	}
	return out
}

func unico(t *testing.T, dir string) string {
	t.Helper()
	var found []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("%d files in %s, want 1: %v", len(found), dir, found)
	}
	return found[0]
}

func TestFilesWritesNDJSON(t *testing.T) {
	dir := t.TempDir()

	res, err := Files{Path: dir + "/"}.Write(context.Background(), lote(3), core.WriteOptions{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.RowsLoaded != 3 {
		t.Errorf("RowsLoaded = %d", res.RowsLoaded)
	}

	body, err := os.ReadFile(unico(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d rows, want 3", len(lines))
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("the first line is not JSON: %v", err)
	}
	if row["sku"] != "A" {
		t.Errorf("the record did not survive: %v", row)
	}
}

// Nothing is added: the record comes out as Transform left it, the same on every
// destination.
func TestFilesAddsNothingWithoutMetadata(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Files{Path: dir + "/"}).Write(context.Background(), lote(1), core.WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	body, _ := os.ReadFile(unico(t, dir))
	var row map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(string(body))), &row)

	if len(row) != 2 {
		t.Errorf("the record has %d fields, want the caller's 2: %v", len(row), row)
	}
	for _, forbidden := range []string{"ingestion_id", "provider", "entity", "source_key", "payload"} {
		if _, tem := row[forbidden]; tem {
			t.Errorf("the SDK wrote %q without being asked", forbidden)
		}
	}
}

func TestFilesComprime(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Files{Path: dir + "/", Compress: true}).
		Write(context.Background(), lote(2), core.WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	path := unico(t, dir)
	if !strings.HasSuffix(path, ".gz") {
		t.Errorf("the file did not end in .gz: %s", path)
	}
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := gzip.NewReader(f); err != nil {
		t.Errorf("the content is not gzip: %v", err)
	}
}

// A directory has no key to match on, and a flag ignored in silence is
// worse than an error.
func TestFilesRefusesAFormatItDoesNotWrite(t *testing.T) {
	_, err := Files{Path: t.TempDir() + "/", Format: "parquet"}.
		Write(context.Background(), lote(1), core.WriteOptions{})
	if err == nil {
		t.Fatal("to.Files writes NDJSON and CSV; parquet has to be refused")
	}
	if !strings.Contains(err.Error(), "parquet") {
		t.Errorf("the error must name the format: %v", err)
	}
}

// The declaration holds on every destination, not only on BigQuery.
func TestFilesChecksColumns(t *testing.T) {
	_, err := Files{Path: t.TempDir() + "/"}.Write(context.Background(), lote(1),
		core.WriteOptions{Columns: []string{"sku", "quantidade", "faltando"}})
	if err == nil {
		t.Fatal("a declared column that is not delivered has to be an error here too")
	}
	if !strings.Contains(err.Error(), "faltando") {
		t.Errorf("the error must name the column: %v", err)
	}
}

func TestFilesWritesCSVWithTheUnionOfTheFields(t *testing.T) {
	dir := t.TempDir()
	records := []core.Envelope{
		{Payload: map[string]any{"a": 1, "b": 2}},
		{Payload: map[string]any{"a": 3}}, // no "b"
	}
	if _, err := (Files{Path: dir + "/", Format: core.FormatCSV}).
		Write(context.Background(), records, core.WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	body, _ := os.ReadFile(unico(t, dir))
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if lines[0] != "a,b" {
		t.Errorf("header = %q, want the sorted union", lines[0])
	}
	if lines[2] != "3," {
		t.Errorf("the row without b = %q; the missing field has to stay empty in the right column", lines[2])
	}
}

// Two batches do not overwrite each other: a directory has no notion of "the
// same rows again".
func TestFilesDoesNotOverwriteThePreviousBatch(t *testing.T) {
	dir := t.TempDir()
	d := Files{Path: dir + "/"}
	for i := 0; i < 2; i++ {
		if _, err := d.Write(context.Background(), lote(1), core.WriteOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("%d files after two loads, want 2", len(entries))
	}
}

// No temporary file is left behind: the write is temp + rename, and the rename
// is atomic on the same filesystem.
func TestFilesLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Files{Path: dir + "/"}).Write(context.Background(), lote(1), core.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".brevis-") {
			t.Errorf("a temporary file was left: %s", e.Name())
		}
	}
}

func TestFilesRefusesAPathWithNoStore(t *testing.T) {
	_, err := Files{Path: "gs://b/x/"}.Write(context.Background(), lote(1), core.WriteOptions{})
	if err == nil {
		t.Fatal("a gs:// path without a Store cannot be written")
	}
	if !strings.Contains(err.Error(), "gs") || !strings.Contains(err.Error(), "Store") {
		t.Errorf("the error must name both: %v", err)
	}
}

// A directory has no key to match on, and a flag ignored in silence is worse
// than an error.
func TestFilesRefusesDedup(t *testing.T) {
	_, err := Files{Path: t.TempDir() + "/"}.Write(context.Background(), lote(1),
		core.WriteOptions{Dedup: core.DedupMerge})
	if err == nil {
		t.Fatal("to.Files cannot deduplicate; accepting the flag would mean ignoring it")
	}
	for _, quer := range []string{"to.Files", "Dedup"} {
		if !strings.Contains(err.Error(), quer) {
			t.Errorf("the error must name %q: %v", quer, err)
		}
	}
}

// The partitioning reads the column the chain composed, not one the
// destination
// acrescenta.
func TestFilesPartitionsByTheRowsColumn(t *testing.T) {
	dir := t.TempDir()
	records := []core.Envelope{{Payload: map[string]any{
		"sku": "W-1", "ingestion_loaded_at": "2026-09-04T10:00:00Z",
	}}}

	if _, err := (Files{Path: dir + "/", PartitionBy: "ingestion_loaded_at"}).
		Write(context.Background(), records, core.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if path := unico(t, dir); !strings.Contains(path, "ingestion_loaded_at=2026-09-04") {
		t.Errorf("the path was not partitioned: %s", path)
	}
}
