package api

import (
	"net/http"
	"time"

	"brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/metrics"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
)

// registerOverviewAndRunsAPI wires the JSON the React Overview and Runs
// screens read. One entry in Registrar; the screens' handlers live in this
// file and in runs_api.go.
func (u *UI) registerOverviewAndRunsAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/overview", u.overviewAPI)
	mux.HandleFunc("GET /api/overview/attention", u.overviewAttentionAPI)
	mux.HandleFunc("GET /api/runs", u.runsAPI)
}

// apiError is a failure the app can show: a JSON body, never an HTML page.
func apiError(w http.ResponseWriter, status int, code apiCode) {
	w.Header().Set("Cache-Control", "no-store")
	fail(w, status, code)
}

type indicatorsView struct {
	Total          int   `json:"total"`
	Succeeded      int   `json:"succeeded"`
	Failed         int   `json:"failed"`
	Running        int   `json:"running"`
	Pending        int   `json:"pending"`
	Retried        int   `json:"retried"`
	MeanDurationMs int64 `json:"meanDurationMs"`
}

type bucketView struct {
	Start     time.Time `json:"start"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	Running   int       `json:"running"`
	Queued    int       `json:"queued"`
}

type nextRunView struct {
	Workflow string    `json:"workflow"`
	Cron     string    `json:"cron"`
	CronText *i18n.Msg `json:"cronText"`
	Timezone string    `json:"timezone"`
	When     time.Time `json:"when"`
	Logos    []string  `json:"logos"`
}

// eventView is one line of the event log. Source is "workflow", "stream" or "endpoint"; What is the
// sentence around the subject, which the app writes in code style where its language puts it.
type eventView struct {
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	What    i18n.Msg  `json:"what"`
	Subject string    `json:"subject"`
	Actor   string    `json:"actor"`
	Href    string    `json:"href"`
	State   string    `json:"state"`
}

type overviewView struct {
	// Window is the key asked for after the fallback (anything unknown is
	// 24h); Grain is "hour" or "day".
	Window     string         `json:"window"`
	Grain      string         `json:"grain"`
	Now        time.Time      `json:"now"`
	Indicators indicatorsView `json:"indicators"`
	// Queued is the runs waiting for a worker right now, not in the window.
	Queued  int           `json:"queued"`
	Buckets []bucketView  `json:"buckets"`
	Next    []nextRunView `json:"next"`
	Recent  []runView     `json:"recent"`
	Events  []eventView   `json:"events"`
}

// overviewAPI is the Overview's numbers, chart, next triggers, recent runs
// and event log. "Needs your attention" is its own endpoint: it follows
// itself on a faster clock than the rest.
func (u *UI) overviewAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	window := pages.ParseWindow(r.URL.Query().Get("window"))
	fail := func(what string, err error) {
		u.log.Error("reading the overview", "what", what, "error", err)
		apiError(w, http.StatusInternalServerError, codeUnavailable)
	}
	if u.reads == nil {
		apiError(w, http.StatusServiceUnavailable, codeUnavailable)
		return
	}

	indicators, err := u.reads.Indicators(ctx, window.Span)
	if err != nil {
		fail("indicators", err)
		return
	}
	buckets, err := u.reads.RunsPer(ctx, window.Grain, window.Buckets)
	if err != nil {
		fail("buckets", err)
		return
	}
	recent, err := u.reads.LatestRuns(ctx, 20)
	if err != nil {
		fail("recent runs", err)
		return
	}
	pending, _, err := u.reads.QueueDepth(ctx)
	if err != nil {
		fail("queue", err)
		return
	}
	schedules, err := u.reads.Schedules(ctx)
	if err != nil {
		fail("schedules", err)
		return
	}

	now := time.Now()
	out := overviewView{
		Window: window.Key, Grain: "hour", Now: now, Queued: pending,
		Indicators: indicatorsView{
			Total: indicators.Total, Succeeded: indicators.Succeeded, Failed: indicators.Failed, Running: indicators.Running,
			Pending: indicators.Pending, Retried: indicators.Retried, MeanDurationMs: indicators.MeanDuration.Milliseconds(),
		},
		Buckets: make([]bucketView, 0, len(buckets)),
		Next:    []nextRunView{},
		Events:  []eventView{},
	}
	if window.Grain == postgres.ByDay {
		out.Grain = "day"
	}
	for _, b := range buckets {
		out.Buckets = append(out.Buckets, bucketView{Start: b.Start, Succeeded: b.Succeeded, Failed: b.Failed, Running: b.Running, Queued: b.Queued})
	}
	for _, n := range u.withLogos(ctx, nextRuns(schedules, now, 8, u.log)) {
		out.Next = append(out.Next, nextRunView{
			Workflow: n.Workflow, Cron: n.Cron, CronText: pages.CronMsg(n.Cron),
			Timezone: n.Timezone, When: n.When, Logos: orEmpty(n.Logos),
		})
	}
	out.Recent = runViews(recent, u.runsLogos(ctx, recent))
	for _, l := range pages.FeedOf(u.eventLog(ctx)) {
		out.Events = append(out.Events, eventView{
			At: l.At, Source: l.Source, What: l.What, Subject: l.Subject,
			Actor: l.Actor, Href: l.Href, State: l.State,
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type causeView struct {
	Step string `json:"step"`
	// MapIndex is null for a step that is not mapped.
	MapIndex *int   `json:"mapIndex"`
	ExitCode *int   `json:"exitCode"`
	Line     string `json:"line"`
}

type attentionItemView struct {
	Workflow   string            `json:"workflow"`
	RunID      string            `json:"runId"`
	FailedAt   time.Time         `json:"failedAt"`
	Streak     int               `json:"streak"`
	Cause      *causeView        `json:"cause"`
	Rerunnable bool              `json:"rerunnable"`
	Params     map[string]string `json:"params"`
	// Rerunning is the id of the workflow's run in flight, "" when none.
	Rerunning string `json:"rerunning"`
}

type attentionTargetView struct {
	Target   string    `json:"target"`
	Status   string    `json:"status"`
	Workflow string    `json:"workflow"`
	Missed   time.Time `json:"missed"`
}

type attentionView struct {
	// Total is everything that needs the reader, shown or not.
	Total       int                   `json:"total"`
	Items       []attentionItemView   `json:"items"`
	More        int                   `json:"more"`
	Targets     []attentionTargetView `json:"targets"`
	TargetsMore int                   `json:"targetsMore"`
	// Streams are the ones whose measured health is failing, worst-first by
	// time; the page follows them on the `metrics:streams` topic.
	Streams     []attentionStreamView `json:"streams"`
	StreamsMore int                   `json:"streamsMore"`
}

type attentionStreamView struct {
	Gateway string     `json:"gateway"`
	Path    string     `json:"path"`
	Href    string     `json:"href"`
	Since   *time.Time `json:"since"`
}

// attentionStreamsShown is how many failing streams the section lists.
const attentionStreamsShown = 5

// overviewAttentionAPI is "Needs your attention", the section the page
// follows every ten seconds.
func (u *UI) overviewAttentionAPI(w http.ResponseWriter, r *http.Request) {
	a := u.attention(r.Context())
	out := attentionView{
		Total: a.Total(), More: a.More, TargetsMore: a.TargetsMore,
		Items: []attentionItemView{}, Targets: []attentionTargetView{}, Streams: []attentionStreamView{},
	}
	out.Streams, out.StreamsMore = u.failingStreams(r)
	out.Total += len(out.Streams) + out.StreamsMore
	for _, it := range a.Items {
		v := attentionItemView{
			Workflow: it.Workflow, RunID: it.RunID.String(), FailedAt: it.FailedAt, Streak: it.Streak,
			Rerunnable: it.Rerunnable, Params: it.Params, Rerunning: it.Rerunning,
		}
		if v.Params == nil {
			v.Params = map[string]string{}
		}
		if c := it.Cause; c != nil {
			v.Cause = &causeView{Step: c.Step, ExitCode: c.ExitCode, Line: c.Line}
			if c.MapIndex != run.Unmapped {
				idx := c.MapIndex
				v.Cause.MapIndex = &idx
			}
		}
		out.Items = append(out.Items, v)
	}
	for _, tg := range a.Targets {
		out.Targets = append(out.Targets, attentionTargetView{Target: tg.Target, Status: string(tg.Status), Workflow: tg.Workflow, Missed: tg.Missed})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// failingStreams are the streams whose measured health is failing: the same
// rule as the Streams list, read from the same snapshot. A console with no
// metrics source has none, which is not the same as "all is well" -- the
// Streams screen says why; here there is nothing to act on.
func (u *UI) failingStreams(r *http.Request) ([]attentionStreamView, int) {
	entries, err := u.streamEntries(r)
	if err != nil {
		u.log.Warn("the failing streams are unavailable", "error", err)
		return []attentionStreamView{}, 0
	}
	v := pages.BuildStreamsWith(entries, pages.StreamFilter{}, u.streamHealth(u.healthSnapshot(r)))
	out := []attentionStreamView{}
	more := 0
	for _, s := range v.Rows {
		h := v.Healths[s.Key()]
		if h.State != metrics.HealthFailing {
			continue
		}
		if len(out) == attentionStreamsShown {
			more++
			continue
		}
		out = append(out, attentionStreamView{Gateway: s.Gateway, Path: s.Path, Href: "/ingest/streams/" + s.Gateway + s.Path, Since: h.Since})
	}
	return out, more
}
