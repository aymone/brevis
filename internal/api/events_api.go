package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"brevis.sh/brevis/internal/events"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// WithChangeHub turns on GET /api/events. Nil leaves the route out, so a
// process (or a test) without a hub serves no stream.
func (u *UI) WithChangeHub(h *events.Hub) *UI {
	u.hub = h
	return u
}

func (u *UI) registerEvents(mux *http.ServeMux) {
	if u.hub != nil {
		mux.HandleFunc("GET /api/events", u.eventsHandler(keepaliveEvery))
	}
}

const (
	// keepaliveEvery is under every proxy idle timeout we know of (nginx
	// defaults to 60 s) and above the detector tick, so it is never the thing
	// that keeps a busy stream alive.
	keepaliveEvery = 20 * time.Second
	// writeBudget bounds ONE write. The server-wide WriteTimeout (30 s) would
	// kill a healthy stream, so it is cleared; but with no deadline at all a
	// client that stops reading would pin this goroutine until TCP gives up.
	// Each write gets a fresh deadline instead.
	writeBudget = 10 * time.Second
)

// eventsHandler serves the stream. keepalive is a parameter for the tests.
func (u *UI) eventsHandler(keepalive time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topics, err := events.ParseTopics(r.URL.Query().Get("topics"))
		if err != nil {
			eventsError(w, http.StatusBadRequest, err)
			return
		}
		rc := http.NewResponseController(w)
		sub, err := u.hub.Subscribe(r.Context(), topics)
		if err != nil {
			if errors.Is(err, events.ErrFull) {
				w.Header().Set("Retry-After", "5")
				eventsError(w, http.StatusServiceUnavailable, err)
			} else if r.Context().Err() == nil {
				eventsError(w, http.StatusServiceUnavailable, err)
			}
			return
		}
		// Frees the hub's slot the moment the handler ends, which includes the
		// client going away (r.Context()).
		defer sub.Close()

		// Both deadlines: the read one too, or ReadTimeout would close the
		// connection under a long-lived response on some transports.
		_ = rc.SetReadDeadline(time.Time{})
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform")
		h.Set("X-Accel-Buffering", "no") // nginx must not buffer the stream
		w.WriteHeader(http.StatusOK)

		write := func(s string) bool {
			_ = rc.SetWriteDeadline(time.Now().Add(writeBudget))
			if _, err := fmt.Fprint(w, s); err != nil {
				return false
			}
			return rc.Flush() == nil
		}
		ready, _ := json.Marshal(map[string][]string{"topics": topics})
		if !write("retry: 3000\n\nevent: ready\ndata: " + string(ready) + "\n\n") {
			return
		}

		tick := time.NewTicker(keepalive)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				if !write(": keepalive\n\n") {
					return
				}
			case t, ok := <-sub.C:
				if !ok {
					return // dropped or shutting down: the browser reconnects
				}
				sub.Ack(t)
				data, _ := json.Marshal(map[string]string{"topic": t})
				if !write("event: change\ndata: " + string(data) + "\n\n") {
					return
				}
			}
		}
	}
}

func eventsError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	msg := strings.TrimPrefix(err.Error(), events.ErrTopics.Error()+": ")
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// NewFingerprints is the hub's factory over the database: one detector per
// topic, each a single indexed query (see postgres.Fingerprints).
func NewFingerprints(fp *postgres.Fingerprints) events.Factory {
	return func(topic string) events.Detector {
		switch {
		case topic == events.Runs || topic == events.Nav || topic == events.Overview:
			// The attention list behind Overview is derived from runs; the heavy
			// FailingWorkflows query is deliberately not run once a second.
			return events.DetectorFunc(fp.Runs)
		case topic == events.Workflows:
			return events.DetectorFunc(fp.Workflows)
		case topic == events.Catalog || topic == events.Ingest:
			return events.DetectorFunc(fp.Data)
		case strings.HasPrefix(topic, "run:"):
			id := topic[4:]
			return events.DetectorFunc(func(ctx context.Context) (string, error) { return fp.Run(ctx, id) })
		default: // workflow:<slug>
			slug := strings.TrimPrefix(topic, "workflow:")
			return events.DetectorFunc(func(ctx context.Context) (string, error) { return fp.Workflow(ctx, slug) })
		}
	}
}
