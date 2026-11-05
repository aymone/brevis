package sdk

import (
	"fmt"
	"testing"
)

// typicalChain is the chain a real fetcher writes: clean up, rename, stamp
// provenance and compose the ingestion_id.
func typicalChain() []Transformer {
	return []Transformer{
		Accept("id", "name", "value", "ts"),
		Rename(map[string]string{"id": "source_key", "ts": "record_ts"}),
		Compute("provider", func(map[string]any) (any, error) { return "bench", nil }),
		Compute("entity", func(map[string]any) (any, error) { return "records", nil }),
		IngestionID(),
		IngestionLoadedAt(),
	}
}

func record(i int) map[string]any {
	return map[string]any{
		"id":    fmt.Sprint(i),
		"name":  "any record",
		"value": 10.5,
		"ts":    "2026-09-05T12:00:00Z",
	}
}

// BenchmarkTransformChain measures the chain alone, with no HTTP and no
// decoding -- the previous benchmark mixed all three, and the profile needed a
// second look to tell which cost belonged to what.
func BenchmarkTransformChain(b *testing.B) {
	fns := typicalChain()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		payload, _, err := applyAll(fns, record(i))
		if err != nil {
			b.Fatal(err)
		}
		if payload == nil {
			b.Fatal("empty payload")
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "records/s")
}

// BenchmarkOneTransformerAlone isolates the cost of ONE map copy, so the
// arithmetic of "how many copies the chain makes" is checkable rather than
// inferred.
func BenchmarkOneTransformerAlone(b *testing.B) {
	fns := []Transformer{Compute("provider", func(map[string]any) (any, error) { return "x", nil })}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := applyAll(fns, record(i)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIngestionID isolates the id's composition, which the profile pointed
// at as the
// biggest block by itself.
func BenchmarkIngestionID(b *testing.B) {
	fns := []Transformer{IngestionID()}
	r := map[string]any{
		"provider": "acme", "entity": "orders",
		"source_key": "12345", "record_ts": "2026-09-05T12:00:00Z",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := make(map[string]any, len(r))
		for k, v := range r {
			copied[k] = v
		}
		if _, _, err := applyAll(fns, copied); err != nil {
			b.Fatal(err)
		}
	}
}
