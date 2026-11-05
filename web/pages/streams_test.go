package pages

import (
	"context"
	"testing"

	"brevis.sh/brevis/web/i18n"
)

// A BATCH IS SAID IN WORDS, in each language: every, up to, or both.
func TestAStreamsBatchIsInWords(t *testing.T) {
	for lang, want := range map[string][3]string{
		"en":    {"2 minutes or 2,000", "30 seconds", "up to 500"},
		"pt-BR": {"2 minutos ou 2.000", "30 segundos", "até 500"},
		"es":    {"2 minutos o 2.000", "30 segundos", "hasta 500"},
	} {
		ctx := i18n.WithLang(context.Background(), lang)
		for i, s := range []Stream{
			{Config: sourceConfig(field("buffer.flush.every", "2m"), field("buffer.flush.records", 2000.0))},
			{Config: sourceConfig(field("buffer.flush.every", "30s"))},
			{Config: sourceConfig(field("buffer.flush.records", 500.0))},
		} {
			if got := s.BatchIn(ctx); got != want[i] {
				t.Errorf("%s batch %d = %q, want %q", lang, i, got, want[i])
			}
		}
	}
	if got := (Stream{}).BatchIn(context.Background()); got != "" {
		t.Errorf("no buffer declared says %q", got)
	}
}

// THE COMPACT BATCH is the declared values, not words: the app writes "2m · 2,000" from them in any language.
func TestAStreamRowCarriesTheDeclaredBatch(t *testing.T) {
	ctx := context.Background()
	row := streamRowJSON(ctx, Stream{Config: sourceConfig(field("buffer.flush.every", "2m"), field("buffer.flush.records", 2000.0))}, StreamHealth{})
	if row.BatchEvery != "2m" || row.BatchRecords == nil || *row.BatchRecords != 2000 {
		t.Errorf("batch = %q, %v", row.BatchEvery, row.BatchRecords)
	}
	if none := streamRowJSON(ctx, Stream{}, StreamHealth{}); none.BatchEvery != "" || none.BatchRecords != nil {
		t.Error("no buffer declared says a batch")
	}
}
