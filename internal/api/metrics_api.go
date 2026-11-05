package api

import (
	"context"
	"net/http"
	"strconv"

	"brevis.sh/brevis/internal/events"
	"brevis.sh/brevis/internal/metrics"
	"brevis.sh/brevis/web/pages"
)

// WithMetrics turns on the live volume endpoints and, with WithMetricsTopics,
// their SSE topics. Nil (the default) serves the same JSON saying "not
// configured", so a screen can tell an absent source from a failed one.
func (u *UI) WithMetrics(c metrics.SnapshotReader) *UI {
	u.scrape = c
	return u
}

// snapshot reads the kind's live volume. With no reader it says "not
// configured", the same JSON a console with no source gets.
func (u *UI) snapshot(ctx context.Context, k metrics.Kind, minutes int, only *[2]string) (metrics.Snapshot, error) {
	if u.scrape == nil {
		return (*metrics.Collector)(nil).Snapshot(k, minutes, only), nil
	}
	return u.scrape.Read(ctx, k, minutes, only)
}

// The paths. Streams live under /api/ingest/metrics/streams because Go's mux
// takes a `{path...}` wildcard only at the END of a pattern, so
// `/streams/{gateway}/{path...}/metrics` cannot be written.
func (u *UI) registerMetricsAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ingest/metrics/streams", u.apiStreamsMetrics)
	mux.HandleFunc("GET /api/ingest/metrics/streams/{gateway}/{path...}", u.apiStreamMetrics)
	mux.HandleFunc("GET /api/endpoints/metrics", u.apiEndpointsMetrics)
	mux.HandleFunc("GET /api/endpoints/{api}/{name}/metrics", u.apiEndpointMetrics)
}

// WithMetricsTopics adds the two topics to the hub's factory. The fingerprint
// comes from the database (metrics.Store.Fingerprint), so every API pod answers
// the same whichever one holds the connection, and it moves only when the
// collector wrote a round, so a change event is never more frequent than data.
func WithMetricsTopics(f events.Factory, st metrics.Store) events.Factory {
	fp := func(k metrics.Kind) events.Detector {
		return events.DetectorFunc(func(ctx context.Context) (string, error) { return st.Fingerprint(ctx, k) })
	}
	return func(topic string) events.Detector {
		switch topic {
		case events.MetricsStreams:
			return fp(metrics.Gateway)
		case events.MetricsEndpoints:
			return fp(metrics.Endpoints)
		}
		return f(topic)
	}
}

// healthSnapshot is the gateways' snapshot the stream health is judged on. A
// failed read is logged and answered as "no source": the list still draws, its
// streams with their static health, rather than the whole screen failing.
func (u *UI) healthSnapshot(r *http.Request) metrics.Snapshot {
	snap, err := u.snapshot(r.Context(), metrics.Gateway, metrics.Retention, nil)
	if err != nil {
		u.log.Warn("the live metrics are unavailable", "error", err)
		return (*metrics.Collector)(nil).Snapshot(metrics.Gateway, metrics.Retention, nil)
	}
	return snap
}

// metricsMinutes reads ?minutes=, bounded by what the collector keeps.
func metricsMinutes(r *http.Request, def int) (int, bool) {
	v := r.URL.Query().Get("minutes")
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	return n, err == nil && n >= 1 && n <= metrics.Retention
}

// streamMetric is a source entity with the console's name for it. Path is
// empty for a stream the source reports and no published config names.
type streamMetric struct {
	metrics.Entity
	Gateway string `json:"gateway"`
	Path    string `json:"path"`
}

type streamsMetricsView struct {
	metrics.Snapshot
	Entities []streamMetric `json:"entities"`
}

// streamNames maps (gateway, stream name as the gateway labels it) to the
// published path, which is what the console's screens key a stream by. The
// name is the `name` field of the stream's published configuration.
func (u *UI) streamNames(r *http.Request) (map[[2]string]string, error) {
	entries, err := u.streamEntries(r)
	if err != nil {
		return nil, err
	}
	out := map[[2]string]string{}
	for _, g := range pages.StreamsOf(entries) {
		for _, s := range g.Streams {
			if n := s.Field("name"); n != "" {
				out[[2]string{g.Name, n}] = s.Path
			}
		}
	}
	return out, nil
}

func (u *UI) apiStreamsMetrics(w http.ResponseWriter, r *http.Request) {
	n, ok := metricsMinutes(r, 30)
	if !ok {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest, "max", metrics.Retention)
		return
	}
	names, err := u.streamNames(r)
	if err != nil {
		u.ingestFail(w, "the streams", err)
		return
	}
	snap, err := u.snapshot(r.Context(), metrics.Gateway, n, nil)
	if err != nil {
		u.log.Error("reading the live metrics", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	v := streamsMetricsView{Snapshot: snap, Entities: make([]streamMetric, 0, len(snap.Entities))}
	for _, e := range snap.Entities {
		v.Entities = append(v.Entities, streamMetric{Entity: e, Gateway: e.Owner, Path: names[[2]string{e.Owner, e.Name}]})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

func (u *UI) apiStreamMetrics(w http.ResponseWriter, r *http.Request) {
	n, ok := metricsMinutes(r, 60)
	if !ok {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest, "max", metrics.Retention)
		return
	}
	entries, err := u.streamEntries(r)
	if err != nil {
		u.ingestFail(w, "the stream", err)
		return
	}
	gateway, path := r.PathValue("gateway"), "/"+r.PathValue("path")
	s := pages.FindStream(pages.StreamsOf(entries), gateway, path)
	if s == nil {
		ingestMissing(w)
		return
	}
	// A stream published without its configuration (a version-1 manifest) has
	// no name to match the gateway's label with: the source cannot be asked
	// about it, and the entity comes back not present.
	name := s.Field("name")
	snap, err := u.snapshot(r.Context(), metrics.Gateway, n, &[2]string{gateway, name})
	if err != nil {
		u.log.Error("reading the live metrics", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	v := streamsMetricsView{Snapshot: snap, Entities: []streamMetric{{Entity: snap.Entities[0], Gateway: gateway, Path: path}}}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

type endpointMetric struct {
	metrics.Entity
	API string `json:"api"`
}

type endpointsMetricsView struct {
	metrics.Snapshot
	Entities []endpointMetric `json:"entities"`
}

func (u *UI) apiEndpointsMetrics(w http.ResponseWriter, r *http.Request) {
	n, ok := metricsMinutes(r, 30)
	if !ok {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest, "max", metrics.Retention)
		return
	}
	snap, err := u.snapshot(r.Context(), metrics.Endpoints, n, nil)
	if err != nil {
		u.log.Error("reading the live metrics", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	v := endpointsMetricsView{Snapshot: snap, Entities: make([]endpointMetric, 0, len(snap.Entities))}
	for _, e := range snap.Entities {
		v.Entities = append(v.Entities, endpointMetric{Entity: e, API: e.Owner})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

func (u *UI) apiEndpointMetrics(w http.ResponseWriter, r *http.Request) {
	n, ok := metricsMinutes(r, 60)
	if !ok {
		dataFail(w, http.StatusBadRequest, codeInvalidRequest, "max", metrics.Retention)
		return
	}
	api, name := r.PathValue("api"), r.PathValue("name")
	if u.endpoints == nil {
		dataFail(w, http.StatusNotFound, codeNotFound)
		return
	}
	all, err := u.endpoints.List(r.Context())
	if err != nil {
		u.log.Error("listing the endpoints", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	found := false
	for _, e := range all {
		if e.API == api && e.Name == name {
			found = true
			break
		}
	}
	if !found {
		dataFail(w, http.StatusNotFound, codeNotFound)
		return
	}
	snap, err := u.snapshot(r.Context(), metrics.Endpoints, n, &[2]string{api, name})
	if err != nil {
		u.log.Error("reading the live metrics", "error", err)
		dataFail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	v := endpointsMetricsView{Snapshot: snap, Entities: []endpointMetric{{Entity: snap.Entities[0], API: api}}}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}
