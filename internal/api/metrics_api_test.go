package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/events"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/metrics"
)

// metricsServer serves one scrape of an exposition that counts upward on
// every request, so two scrapes give a delta.
func metricsServer(t *testing.T, body func(n int) string) *httptest.Server {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		_, _ = w.Write([]byte(body(n)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// scraped runs the collector's loop long enough for two scrapes and stops it.
func scraped(t *testing.T, kind metrics.Kind, owner, url string) *metrics.Collector {
	t.Helper()
	c := metrics.New(metrics.Config{Targets: []metrics.Target{metrics.NewTarget(kind, owner, url)}, Interval: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	deadline := time.Now().Add(6 * time.Second)
	for c.Seq(kind) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if c.Seq(kind) < 2 {
		t.Fatal("the collector did not scrape twice")
	}
	return c
}

func namedStream() catalogFake {
	f := catalogFake{entries: []postgres.CatalogEntry{{Target: "pubsub://acme/clicks", Kind: "pubsub", Writers: []postgres.CatalogWriter{{
		Gateway: &postgres.GatewayWriter{Name: "edge", Stream: "/v1/clicks", Role: "sink"}}}}}}
	f.streams = map[postgres.StreamKey]*postgres.WriterConfig{{Gateway: "edge", Path: "/v1/clicks"}: {
		Fields: []catalog.ManifestField{{Path: "name", Value: "clicks"}}}}
	return f
}

func TestStreamsMetricsNotConfigured(t *testing.T) {
	code, body := ingestJSON(t, dataUI(namedStream()), "/api/ingest/metrics/streams")
	if code != http.StatusOK || body["configured"] != false {
		t.Fatalf("%d %v", code, body)
	}
	if len(list(t, body, "entities")) != 0 || len(list(t, body, "owners")) != 0 {
		t.Fatalf("something invented: %v", body)
	}
	code, body = ingestJSON(t, dataUI(namedStream()), "/api/ingest/metrics/streams/edge/v1/clicks")
	ents := list(t, body, "entities")
	if code != http.StatusOK || len(ents) != 1 || ents[0].(map[string]any)["present"] != false {
		t.Fatalf("%d %v", code, body)
	}
}

func TestStreamsMetricsFromAScrapedGateway(t *testing.T) {
	srv := metricsServer(t, func(n int) string {
		return "# TYPE brevis_gateway_events_received_total counter\n" +
			"brevis_gateway_events_received_total{stream=\"clicks\",format=\"json\"} " + strings.Repeat("1", n) + "\n" +
			"brevis_gateway_events_received_total{stream=\"unpublished\",format=\"json\"} 5\n" +
			"# TYPE brevis_gateway_buffer_records gauge\nbrevis_gateway_buffer_records{stream=\"clicks\"} 3\n"
	})
	c := scraped(t, metrics.Gateway, "edge", srv.URL)
	ui := dataUI(namedStream()).WithMetrics(c)

	code, body := ingestJSON(t, ui, "/api/ingest/metrics/streams?minutes=10")
	if code != http.StatusOK || body["configured"] != true {
		t.Fatalf("%d %v", code, body)
	}
	byName := map[string]map[string]any{}
	for _, e := range list(t, body, "entities") {
		m := e.(map[string]any)
		byName[m["name"].(string)] = m
	}
	if byName["clicks"]["path"] != "/v1/clicks" || byName["clicks"]["gateway"] != "edge" {
		t.Errorf("a published stream is keyed by its path: %v", byName["clicks"])
	}
	if byName["unpublished"]["path"] != "" {
		t.Errorf("a stream no config names must not borrow a path: %v", byName["unpublished"])
	}
	if got := len(byName["clicks"]["values"].(map[string]any)["received"].([]any)); got != 10 {
		t.Errorf("minutes = %d", got)
	}
	if byName["clicks"]["gauges"].(map[string]any)["buffered"] != float64(3) {
		t.Errorf("gauges %v", byName["clicks"]["gauges"])
	}

	_, one := ingestJSON(t, ui, "/api/ingest/metrics/streams/edge/v1/clicks")
	ents := list(t, one, "entities")
	if len(ents) != 1 || ents[0].(map[string]any)["name"] != "clicks" || len(ents[0].(map[string]any)["covered"].([]any)) != 60 {
		t.Fatalf("detail: %v", one)
	}
	if code, _ := ingestJSON(t, ui, "/api/ingest/metrics/streams/edge/v1/nope"); code != http.StatusNotFound {
		t.Errorf("unknown stream = %d", code)
	}
}

func TestMetricsMinutesAreValidated(t *testing.T) {
	ui := dataUI(namedStream()).WithEndpoints(twoAPIs())
	for _, p := range []string{"/api/ingest/metrics/streams", "/api/ingest/metrics/streams/edge/v1/clicks", "/api/endpoints/metrics", "/api/endpoints/sales-api/orders/metrics"} {
		for _, bad := range []string{"0", "181", "x", "-3"} {
			code, body := dataJSON(t, ui, "GET", p+"?minutes="+bad, "", "")
			if code != http.StatusBadRequest || body["error"] == nil {
				t.Errorf("%s?minutes=%s = %d %v", p, bad, code, body)
			}
		}
	}
}

func TestEndpointsMetrics(t *testing.T) {
	ui := dataUI(catalogFake{}).WithEndpoints(twoAPIs())
	code, body := dataJSON(t, ui, "GET", "/api/endpoints/metrics", "", "")
	if code != http.StatusOK || body["configured"] != false {
		t.Fatalf("not configured: %d %v", code, body)
	}
	code, body = dataJSON(t, ui, "GET", "/api/endpoints/sales-api/orders/metrics", "", "")
	if ents := body["entities"].([]any); code != http.StatusOK || len(ents) != 1 || ents[0].(map[string]any)["present"] != false {
		t.Fatalf("known endpoint, no source: %d %v", code, body)
	}
	if code, _ := dataJSON(t, ui, "GET", "/api/endpoints/sales-api/ghost/metrics", "", ""); code != http.StatusNotFound {
		t.Errorf("ghost endpoint = %d", code)
	}
	if code, _ := dataJSON(t, dataUI(catalogFake{}), "GET", "/api/endpoints/sales-api/orders/metrics", "", ""); code != http.StatusNotFound {
		t.Errorf("without endpoints = %d", code)
	}

	srv := metricsServer(t, func(n int) string {
		return "# TYPE brevis_sql_api_requests_total counter\n" +
			"brevis_sql_api_requests_total{endpoint=\"orders\",status=\"200\"} " + strings.Repeat("1", n) + "\n" +
			"brevis_sql_api_requests_total{endpoint=\"orders\",status=\"500\"} " + strings.Repeat("1", n) + "\n" +
			"brevis_sql_api_requests_total{endpoint=\"-\",status=\"401\"} 9\n"
	})
	ui = ui.WithMetrics(scraped(t, metrics.Endpoints, "sales-api", srv.URL))
	_, body = dataJSON(t, ui, "GET", "/api/endpoints/sales-api/orders/metrics?minutes=5", "", "")
	e := body["entities"].([]any)[0].(map[string]any)
	if e["present"] != true || e["api"] != "sales-api" || e["ratePerMin"].(map[string]any)["ok"] == nil {
		t.Fatalf("%v", e)
	}
	_, body = dataJSON(t, ui, "GET", "/api/endpoints/metrics", "", "")
	if n := len(body["entities"].([]any)); n != 1 {
		t.Errorf("the unattributed '-' became an endpoint: %d entities", n)
	}
}

// fpStore is a metrics.Store that only knows fingerprints.
type fpStore struct{ fp map[metrics.Kind]string }

func (f *fpStore) Write(context.Context, metrics.Export) error { return nil }
func (f *fpStore) Load(context.Context, metrics.Kind, int64) (metrics.Rows, error) {
	return metrics.Rows{}, nil
}
func (f *fpStore) Prune(context.Context, int64, int) (int, error)                { return 0, nil }
func (f *fpStore) Fingerprint(_ context.Context, k metrics.Kind) (string, error) { return f.fp[k], nil }

func TestMetricsTopicsFingerprintComesFromTheStore(t *testing.T) {
	var base int
	st := &fpStore{fp: map[metrics.Kind]string{metrics.Gateway: "g1", metrics.Endpoints: "e1"}}
	factory := api.WithMetricsTopics(func(string) events.Detector {
		base++
		return events.DetectorFunc(func(context.Context) (string, error) { return "db", nil })
	}, st)
	for topic, want := range map[string]string{events.MetricsStreams: "g1", events.MetricsEndpoints: "e1"} {
		fp, err := factory(topic).Fingerprint(context.Background())
		if err != nil || fp != want {
			t.Errorf("%s: %q %v", topic, fp, err)
		}
	}
	if base != 0 {
		t.Error("the metrics topics fell through to the database detector")
	}
	st.fp[metrics.Gateway] = "g2"
	if fp, _ := factory(events.MetricsStreams).Fingerprint(context.Background()); fp != "g2" {
		t.Error("the fingerprint did not follow the store")
	}
	if fp, _ := factory(events.MetricsEndpoints).Fingerprint(context.Background()); fp != "e1" {
		t.Error("the endpoints topic moved with the gateway's rows")
	}
	if fp, _ := factory(events.Runs).Fingerprint(context.Background()); fp != "db" || base != 1 {
		t.Error("other topics must keep their detector")
	}
}

// The streams list and the detail say a health derived from the scraped series,
// the counts agree with the rows, and with no source the reason says so.
func TestStreamHealthFromTheSource(t *testing.T) {
	// No source: unknown, and the reason names it.
	_, body := ingestJSON(t, dataUI(namedStream()), "/api/ingest/streams")
	row := list(t, body, "rows")[0].(map[string]any)
	if h := row["health"].(map[string]any); h["state"] != "unknown" || h["reason"] != "source_not_configured" {
		t.Errorf("no source: %v", h)
	}

	// A gateway counting up with a delivery per scrape: healthy.
	srv := metricsServer(t, func(n int) string {
		return "brevis_gateway_events_received_total{stream=\"clicks\",format=\"json\"} " + strings.Repeat("1", n) + "\n" +
			"brevis_gateway_batches_total{stream=\"clicks\",sink=\"x\",outcome=\"delivered\"} " + strings.Repeat("1", n) + "\n"
	})
	ui := dataUI(namedStream()).WithMetrics(scraped(t, metrics.Gateway, "edge", srv.URL))
	_, body = ingestJSON(t, ui, "/api/ingest/streams")
	row = list(t, body, "rows")[0].(map[string]any)
	if h := row["health"].(map[string]any); h["state"] != "healthy" || h["reason"] != "delivering" || h["windowMinutes"] != float64(5) {
		t.Errorf("list: %v", h)
	}
	if c := body["healthCounts"].(map[string]any); c["healthy"] != float64(1) || len(c) != 1 {
		t.Errorf("counts: %v", c)
	}
	_, one := ingestJSON(t, ui, "/api/ingest/streams/edge/v1/clicks")
	if h := one["health"].(map[string]any); h["state"] != "healthy" {
		t.Errorf("detail: %v", h)
	}
}
