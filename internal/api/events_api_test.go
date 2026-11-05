package api

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"brevis.sh/brevis/internal/events"
)

// stream serves the real handler on a real http.Server with the production
// timeouts (WriteTimeout shortened by the caller), because the 30 s write
// deadline is exactly what an httptest recorder cannot reproduce.
func stream(t *testing.T, max int, keepalive, writeTimeout time.Duration) (url string, hub *events.Hub, val *atomic.Value, stop context.CancelFunc) {
	t.Helper()
	val = &atomic.Value{}
	val.Store("v1")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hub = events.NewHub(ctx, func(string) events.Detector {
		return events.DetectorFunc(func(context.Context) (string, error) { return val.Load().(string), nil })
	}, events.Options{Interval: 10 * time.Millisecond, MaxSubs: max})
	u := &UI{hub: hub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", u.eventsHandler(keepalive))
	// Through Server.ServeHTTP, as in production: its access-log wrapper is
	// what once hid Flush and the deadlines from this handler.
	srv := httptest.NewUnstartedServer(&Server{log: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: mux})
	srv.Config.WriteTimeout = writeTimeout
	srv.Config.ReadTimeout = 30 * time.Second
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL + "/api/events", hub, val, cancel
}

func open(t *testing.T, url string) (*http.Response, *bufio.Reader, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = resp.Body.Close() })
	return resp, bufio.NewReader(resp.Body), cancel
}

// block reads up to the blank line that ends one SSE message.
func block(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended after %q: %v", b.String(), err)
		}
		b.WriteString(line)
		if line == "\n" {
			return b.String()
		}
	}
}

func TestEventsStreamReadyThenChangeThenKeepalive(t *testing.T) {
	url, _, val, _ := stream(t, 10, 150*time.Millisecond, 30*time.Second)
	resp, r, _ := open(t, url+"?topics=runs,nav,runs")
	for k, want := range map[string]string{
		"Content-Type": "text/event-stream", "Cache-Control": "no-cache, no-transform",
		"X-Accel-Buffering": "no", "Content-Encoding": "",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := block(t, r); got != "retry: 3000\n\n" {
		t.Fatalf("first block = %q", got)
	}
	if got := block(t, r); got != "event: ready\ndata: {\"topics\":[\"runs\",\"nav\"]}\n\n" {
		t.Fatalf("ready = %q", got)
	}
	// No change on connect: the next thing is the keepalive.
	if got := block(t, r); got != ": keepalive\n\n" {
		t.Fatalf("expected a keepalive first, got %q", got)
	}
	val.Store("v2")
	seen := map[string]bool{}
	for len(seen) < 2 {
		got := block(t, r)
		if got == ": keepalive\n\n" {
			continue
		}
		switch got {
		case "event: change\ndata: {\"topic\":\"runs\"}\n\n":
			seen["runs"] = true
		case "event: change\ndata: {\"topic\":\"nav\"}\n\n":
			seen["nav"] = true
		default:
			t.Fatalf("unexpected block %q", got)
		}
	}
}

func TestEventsRefusals(t *testing.T) {
	url, _, _, _ := stream(t, 1, time.Hour, 30*time.Second)
	for _, q := range []string{"", "?topics=", "?topics=bogus", "?topics=run:nope", "?topics=" + tooMany()} {
		resp, err := http.Get(url + q)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 400 || !strings.HasPrefix(string(body), `{"error":"`) ||
			resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%q: %d %s %q", q, resp.StatusCode, resp.Header.Get("Content-Type"), body)
		}
	}

	_, r, _ := open(t, url+"?topics=runs") // takes the only slot
	block(t, r)
	resp, err := http.Get(url + "?topics=runs")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 503 || strings.TrimSpace(string(body)) != `{"error":"too many live connections"}` {
		t.Errorf("over the cap: %d %q", resp.StatusCode, body)
	}
}

func TestEventsClientDisconnectFreesTheSubscription(t *testing.T) {
	url, hub, _, _ := stream(t, 10, time.Hour, 30*time.Second)
	_, r, cancel := open(t, url+"?topics=runs")
	block(t, r)
	if s, d := hub.Stats(); s != 1 || d != 1 {
		t.Fatalf("subs=%d detectors=%d while connected", s, d)
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, d := hub.Stats(); s == 0 && d == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("subscription still held after the client left")
}

// THE SERVER'S 30 s WriteTimeout WOULD END A HEALTHY STREAM. Proved at 300 ms:
// the stream must still deliver well past it.
func TestEventsOutliveTheServerWriteTimeout(t *testing.T) {
	url, _, val, _ := stream(t, 10, 50*time.Millisecond, 300*time.Millisecond)
	_, r, _ := open(t, url+"?topics=runs")
	block(t, r)
	block(t, r)
	start := time.Now()
	for time.Since(start) < 900*time.Millisecond { // three times the timeout, in keepalives
		if got := block(t, r); got != ": keepalive\n\n" {
			t.Fatalf("got %q", got)
		}
	}
	val.Store("v2")
	if got := block(t, r); got != "event: change\ndata: {\"topic\":\"runs\"}\n\n" {
		t.Fatalf("change after the timeout = %q", got)
	}
}

func TestEventsShutdownEndsTheStream(t *testing.T) {
	url, _, _, shutdown := stream(t, 10, time.Hour, 30*time.Second)
	_, r, _ := open(t, url+"?topics=runs")
	block(t, r)
	block(t, r)
	shutdown()
	done := make(chan error, 1)
	go func() { _, err := r.ReadByte(); done <- err }()
	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("read = %v, want EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream survived the shutdown")
	}
}

// tooMany is 21 DISTINCT topics: duplicates are folded, so repeating one does not count.
func tooMany() string {
	var l []string
	for i := 0; i < 21; i++ {
		l = append(l, "workflow:w"+strings.Repeat("x", i))
	}
	return strings.Join(l, ",")
}
