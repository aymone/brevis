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
func (u *UI) WithMetrics(c *metrics.Collector) *UI {
	u.scrape = c
	return u
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
// is the collector's scrape counter of the kind: it changes once per finished
// scrape, so a change event cannot be emitted more often than data arrives,
// and it never changes (never emits) when nothing is configured.
func WithMetricsTopics(f events.Factory, c *metrics.Collector) events.Factory {
	seq := func(k metrics.Kind) events.Detector {
		return events.DetectorFunc(func(context.Context) (string, error) {
			return strconv.FormatUint(c.Seq(k), 10), nil
		})
	}
	return func(topic string) events.Detector {
		switch topic {
		case events.MetricsStreams:
			return seq(metrics.Gateway)
		case events.MetricsEndpoints:
			return seq(metrics.Endpoints)
		}
		return f(topic)
	}
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
		dataFail(w, http.StatusBadRequest, "minutes must be a number from 1 to "+strconv.Itoa(metrics.Retention))
		return
	}
	names, err := u.streamNames(r)
	if err != nil {
		u.ingestFail(w, "the streams", err)
		return
	}
	snap := u.scrape.Snapshot(metrics.Gateway, n, nil)
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
		dataFail(w, http.StatusBadRequest, "minutes must be a number from 1 to "+strconv.Itoa(metrics.Retention))
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
		ingestMissing(w, "that stream")
		return
	}
	// A stream published without its configuration (a version-1 manifest) has
	// no name to match the gateway's label with: the source cannot be asked
	// about it, and the entity comes back not present.
	name := s.Field("name")
	snap := u.scrape.Snapshot(metrics.Gateway, n, &[2]string{gateway, name})
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
		dataFail(w, http.StatusBadRequest, "minutes must be a number from 1 to "+strconv.Itoa(metrics.Retention))
		return
	}
	snap := u.scrape.Snapshot(metrics.Endpoints, n, nil)
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
		dataFail(w, http.StatusBadRequest, "minutes must be a number from 1 to "+strconv.Itoa(metrics.Retention))
		return
	}
	api, name := r.PathValue("api"), r.PathValue("name")
	if u.endpoints == nil {
		dataFail(w, http.StatusNotFound, "that endpoint not found")
		return
	}
	all, err := u.endpoints.List(r.Context())
	if err != nil {
		u.log.Error("listing the endpoints", "error", err)
		dataFail(w, http.StatusInternalServerError, "the endpoints are unavailable")
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
		dataFail(w, http.StatusNotFound, "that endpoint not found")
		return
	}
	snap := u.scrape.Snapshot(metrics.Endpoints, n, &[2]string{api, name})
	v := endpointsMetricsView{Snapshot: snap, Entities: []endpointMetric{{Entity: snap.Entities[0], API: api}}}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}
