package pages

import (
	"testing"

	evkind "brevis.sh/brevis/internal/domain/events"
)

// EVERY KIND THE FEED KNOWS IS A SENTENCE the app says, and a source the app
// names -- a kind with no sentence would draw nothing.
func TestEveryEventKindIsASentence(t *testing.T) {
	for _, k := range evkind.Kinds {
		if m := eventSentence(k); m.K == "" {
			t.Errorf("%s has no sentence", k)
		}
		if source, ok := evkind.SourceOf(k); !ok || sourceCode(source) == "" {
			t.Errorf("%s has no source", k)
		}
	}
}
