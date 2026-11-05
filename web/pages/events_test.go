package pages

import (
	"context"
	"strings"
	"testing"

	evkind "brevis.sh/brevis/internal/domain/events"
	"brevis.sh/brevis/web/i18n"
)

// EVERY KIND THE FEED KNOWS IS A SENTENCE, in every language, with a place
// for what it is about -- a kind with no sentence would draw its key.
func TestEveryEventKindIsASentence(t *testing.T) {
	for _, lang := range []string{"en", "pt-BR", "es"} {
		ctx := i18n.WithLang(context.Background(), lang)
		for _, k := range evkind.Kinds {
			before, after := eventSentence(ctx, k)
			if strings.Contains(before+after, "[") || before+after == "" {
				t.Errorf("%s: %s reads %q · %q", lang, k, before, after)
			}
		}
	}
}
