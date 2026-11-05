package pages

import (
	"testing"
)

// A BATCH IS SENT AS DATA for the app to say: every, up to, or both.
func TestAStreamsBatchIsInWords(t *testing.T) {
	want := [3]string{"2 minutes or 2,000", "30 seconds", "up to 500"}
	for i, s := range []Stream{
		{Config: sourceConfig(field("buffer.flush.every", "2m"), field("buffer.flush.records", 2000.0))},
		{Config: sourceConfig(field("buffer.flush.every", "30s"))},
		{Config: sourceConfig(field("buffer.flush.records", 500.0))},
	} {
		if got := saidEN(s.BatchIn()); got != want[i] {
			t.Errorf("batch %d = %q, want %q", i, got, want[i])
		}
	}
	if got := (Stream{}).BatchIn(); got != nil {
		t.Errorf("no buffer declared says %q", saidEN(got))
	}
}

// THE COMPACT BATCH is the declared values, not words: the app writes "2m · 2,000" from them in any language.
func TestAStreamRowCarriesTheDeclaredBatch(t *testing.T) {
	row := streamRowJSON(Stream{Config: sourceConfig(field("buffer.flush.every", "2m"), field("buffer.flush.records", 2000.0))}, StreamHealth{})
	if row.BatchEvery != "2m" || row.BatchRecords == nil || *row.BatchRecords != 2000 {
		t.Errorf("batch = %q, %v", row.BatchEvery, row.BatchRecords)
	}
	if none := streamRowJSON(Stream{}, StreamHealth{}); none.BatchEvery != "" || none.BatchRecords != nil {
		t.Error("no buffer declared says a batch")
	}
}
