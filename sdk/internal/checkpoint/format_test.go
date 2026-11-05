package checkpoint

import (
	"encoding/json"
	"testing"
)

// TestTheManifestKeysAreTheOnDiskFormat pins the manifest's JSON keys.
//
// A rename here does not fail to compile and does not fail loudly at runtime,
// which is exactly why this test exists.
//
// What it costs, per key:
//
//   - "records": the count check goes silent, and a truncated depot loads
//     fewer rows than the first attempt did;
//   - "parts": every resume finds an empty depot and re-extracts, spending
//     the vendor quota this package exists to save;
//   - "numbers": UseNumber is never turned on, `19.0` comes back as `19`, and
//     every ingestion_id of the resumed attempt differs from the first one's.
//     No error, no log -- a duplicated row after the next merge.
//
// Changing the format is allowed. Changing it by accident is not, and this is
// the line that tells the two apart.
func TestTheManifestKeysAreTheOnDiskFormat(t *testing.T) {
	b, err := json.Marshal(Manifest{
		Version: manifestVersion, Records: 2,
		Parts: []string{"part-00000.ndjson"}, Numbers: NumbersLiteral,
		Pipeline: "p", Run: "r", WrittenAt: "2026-09-06T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"version", "records", "parts", "numbers", "written_at"} {
		if _, ok := got[key]; !ok {
			t.Errorf("the manifest no longer writes %q. Full manifest: %s", key, b)
		}
	}
	if len(got) != 7 {
		t.Errorf("the manifest has %d keys, want 7: %s", len(got), b)
	}

	if manifestFile != "_complete" {
		t.Errorf("the manifest file is %q, want _complete", manifestFile)
	}
	if partPattern != "part-%05d.ndjson" {
		t.Errorf("the part pattern is %q, want part-%%05d.ndjson", partPattern)
	}
}

// A depot written by an SDK before the keys went English holds `_completo` and
// Portuguese keys; it must still be read, or a resume across the upgrade
// re-extracts and spends the vendor's quota again.
func TestAManifestWrittenBeforeTheRenameStillReads(t *testing.T) {
	old := `{"versao":1,"registros":2,"partes":["parte-00000.ndjson"],"numeros":"literal","pipeline":"p","run":"r","gravado_em":"2026-09-06T00:00:00Z"}`
	var m Manifest
	if err := json.Unmarshal([]byte(old), &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != manifestVersion || m.Records != 2 || len(m.Parts) != 1 || m.Parts[0] != "parte-00000.ndjson" ||
		m.Numbers != NumbersLiteral || m.Pipeline != "p" || m.Run != "r" || m.WrittenAt != "2026-09-06T00:00:00Z" {
		t.Errorf("read %+v", m)
	}
	if legacyManifestFile != "_completo" {
		t.Errorf("the legacy manifest file is %q, and an earlier SDK wrote _completo", legacyManifestFile)
	}
}
