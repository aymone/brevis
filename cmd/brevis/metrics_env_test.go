package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/config"
)

// The two lists of metrics addresses were removed. A deployment that still sets
// them must keep starting and be told, once each, that they do nothing now.
func TestTheRemovedMetricsEnvsOnlyWarn(t *testing.T) {
	t.Setenv("BREVIS_GATEWAY_METRICS_URLS", "gw=http://10.0.0.1:9090/metrics")
	t.Setenv("BREVIS_ENDPOINTS_METRICS_URLS", "api=http://10.0.0.2:9090/metrics")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Outside a cluster the collector returns before it touches the pool.
	if err := metricsCollector(ctx, config.Config{}, nil, log); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, name := range []string{"BREVIS_GATEWAY_METRICS_URLS", "BREVIS_ENDPOINTS_METRICS_URLS"} {
		if strings.Count(out, name+" is ignored") != 1 {
			t.Errorf("%s: want one warning in:\n%s", name, out)
		}
	}
	if strings.Contains(out, "10.0.0.") {
		t.Errorf("the warning must not echo the addresses:\n%s", out)
	}
}

func TestNoWarningWhenTheRemovedMetricsEnvsAreUnset(t *testing.T) {
	var buf bytes.Buffer
	warnRemovedMetricsEnv(slog.New(slog.NewTextHandler(&buf, nil)), func(string) string { return "" })
	if buf.Len() != 0 {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}
