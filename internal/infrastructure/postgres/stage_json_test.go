package postgres_test

import (
	"encoding/json"
	"testing"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// A recorded phase is read under both spellings: rows written before the keys
// went English (and by a pod of that engine still running during a rolling
// update) carry indice/nome/estado/em/numeros.
func TestAStageIsReadUnderEitherSpelling(t *testing.T) {
	for name, raw := range map[string]string{
		"english":    `{"index":2,"name":"load","state":"done","ms":900,"at":"x","numbers":{"rows":5}}`,
		"portuguese": `{"indice":2,"nome":"load","estado":"done","ms":900,"em":"x","numeros":{"rows":5}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var s postgres.Stage
			if err := json.Unmarshal([]byte(raw), &s); err != nil {
				t.Fatal(err)
			}
			if s.Index != 2 || s.Name != "load" || s.State != "done" || s.At != "x" ||
				s.Ms == nil || *s.Ms != 900 || s.Numbers["rows"] != 5.0 {
				t.Errorf("read %+v", s)
			}
		})
	}
}

// What the engine writes is English only.
func TestAStageIsWrittenInEnglish(t *testing.T) {
	ms := int64(1)
	b, err := json.Marshal(postgres.Stage{Index: 1, Name: "load", State: "done", Ms: &ms, At: "x", Numbers: map[string]any{"rows": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"index":1,"name":"load","state":"done","ms":1,"at":"x","numbers":{"rows":1}}`; got != want {
		t.Errorf("wrote %s, wanted %s", got, want)
	}
}
