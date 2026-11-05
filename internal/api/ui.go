package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"brevis.sh/brevis/internal/events"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"brevis.sh/brevis/internal/alerts"
	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/branding"
	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/domain/run"
	sch "brevis.sh/brevis/internal/domain/schedule"
	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/internal/infrastructure/endpointcall"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/internal/infrastructure/sqlserve"
	"brevis.sh/brevis/internal/metrics"
	"brevis.sh/brevis/internal/updates"
	"brevis.sh/brevis/web/assets"
	"brevis.sh/brevis/web/i18n"
	"brevis.sh/brevis/web/pages"
	"brevis.sh/brevis/web/spa"
)

// workflowWindow is the horizon of ONE workflow's numbers, and it is thirty
// days rather than the dashboard's twenty-four hours.
//
// The two screens answer different questions. The dashboard is "is the
// installation healthy right now", and a day is the right window for that. A
// workflow's page is "is this pipeline reliable", and a daily job has one run
// in twenty-four hours -- a success rate over a single sample is not a rate.
const workflowWindow = 30 * 24 * time.Hour

// Leitura is what the UI needs from the database. The interface is declared here, in the consumer.
type Leitura interface {
	Indicators(ctx context.Context, window time.Duration) (postgres.Indicators, error)
	IndicatorsFor(ctx context.Context, window time.Duration, workflow string) (postgres.Indicators, error)
	RunsPerDay(ctx context.Context, workflow string, days int) ([]postgres.Day, error)
	LoadTrend(ctx context.Context, workflow string, days int) ([]postgres.LoadDay, error)
	RunsPer(ctx context.Context, g postgres.Grain, n int) ([]postgres.Bucket, error)
	InFlight(ctx context.Context, limite int) ([]postgres.RunSummary, error)
	LatestRuns(ctx context.Context, limite int) ([]postgres.RunSummary, error)
	Runs(ctx context.Context, f postgres.RunFilter) ([]postgres.RunSummary, error)
	CountRuns(ctx context.Context, f postgres.RunFilter) (int, error)
	CountRunsByState(ctx context.Context, f postgres.RunFilter) (map[string]int, error)
	// CountRunsInFlight is the sidebar badge's number; it must stay cheap.
	CountRunsInFlight(ctx context.Context) (int, error)
	// WorkflowHistory is a workflow's runs with their params, newest first;
	// with a day, that UTC day's.
	WorkflowHistory(ctx context.Context, slug string, day *time.Time, limit int) ([]postgres.RunSummary, error)
	Workflows(ctx context.Context) ([]postgres.WorkflowSummary, error)
	// Connects is the schemes each workflow's runs wrote since a moment.
	Connects(ctx context.Context, since time.Time) (map[string][]string, error)
	// LastStepRuns is how each step of a workflow went the last time it ran;
	// StepConnects what each step wrote since a moment.
	LastStepRuns(ctx context.Context, slug string) (map[string]postgres.StepRun, error)
	StepConnects(ctx context.Context, slug string, since time.Time) (map[string][]string, error)
	RecentStates(ctx context.Context, slugs []string, n int) (map[string][]postgres.RecentRun, error)
	Schedules(ctx context.Context) ([]postgres.ScheduleSummary, error)
	Projects(ctx context.Context) ([]postgres.ProjectSummary, error)
	QueueDepth(ctx context.Context) (int, int, error)
}

// Definitions reads a workflow's published definition. Kept apart from `Leitura`
// because it returns the domain, not a screen projection.
type Definitions interface {
	Definition(ctx context.Context, slug string) (wf.Workflow, error)
	// Definitions is every published definition by slug, in one read: what
	// a page with many workflows uses for their logos.
	Definitions(ctx context.Context) (map[string]wf.Workflow, error)
}

// RunsChart reads a Run and the state of its steps.
type RunsChart interface {
	Get(ctx context.Context, id uuid.UUID) (run.Run, error)
	NodeStates(ctx context.Context, id uuid.UUID) (map[string]postgres.NodeState, error)
	LogsDaRun(ctx context.Context, id uuid.UUID) ([]postgres.StepLog, error)
	// WaitingOn says what concurrency limit holds a queued run, if one does.
	WaitingOn(ctx context.Context, id uuid.UUID) (*postgres.Wait, error)
}

// AlertsReader lists a run's alerts. It is a CONSTRUCTOR argument and not a
// settable field, so that forgetting to wire it is a compile error rather than
// a screen that quietly stops showing alerts. Two features shipped switched off
// in one week because nothing outside a test ever set the field they needed.
//
// nil is still allowed, for a process that renders no pages.
type AlertsReader interface {
	ForRun(ctx context.Context, runID uuid.UUID) ([]alerts.Record, error)
}

// CatalogReader lists the destinations the ecosystem writes, for /data. A
// constructor argument for AlertsReader's reason: forgetting to wire it is a
// compile error, not a catalog that quietly shows nothing. nil is allowed for a
// process that renders no pages.
type CatalogReader interface {
	Catalog(ctx context.Context) ([]postgres.CatalogEntry, error)
	CatalogTarget(ctx context.Context, target string) (*postgres.TargetDetail, error)
	StreamConfigs(ctx context.Context) (map[postgres.StreamKey]*postgres.WriterConfig, error)
	StepConfigs(ctx context.Context) (map[postgres.StepKey]*postgres.WriterConfig, error)
}

// Actions are the three effects the screen triggers. A small interface on
// purpose: the UI must not be able to do anything more to the system than
// pause a schedule, ask for a run now, and stop one.
type Actions interface {
	// Toggle pauses or resumes; actor is who pressed it, for the events feed.
	Toggle(ctx context.Context, slug, actor string) (bool, error)
	// Disparar runs a workflow now; with a slot, that past slot of its
	// schedule.
	Disparar(ctx context.Context, slug string, now time.Time, params map[string]string, slot *time.Time) (uuid.UUID, error)
	// Cancel stops a run, or asks the dispatcher holding it to; actor is who
	// pressed it.
	Cancel(ctx context.Context, id uuid.UUID, actor string) (postgres.CancelResult, error)
}

// UI registers the server-rendered pages and the JSON the React island consumes.
type UI struct {
	leitura  Leitura
	inFlight func(context.Context) (int, error)
	// version is what the console runs; updates says if there is newer.
	version string
	updates UpdateSource
	defs    Definitions
	execs   RunsChart
	actions Actions
	alerts  AlertsReader
	catalog CatalogReader
	brand   branding.Brand

	// endpoints is what brevis-sql-api files were published with. Nil turns
	// the pages off.
	endpoints EndpointsReader
	failing   FailingReader

	// attention is the Catalog badge's count, kept for attentionTTL: the
	// catalog is read whole to count it, and every page draws the badge.
	attentionMu sync.Mutex
	attentionAt time.Time
	attentionN  int
	events      EventsReader

	// calls runs an endpoint for Try it; callsProtected says the console
	// asks for a login. Both, or no Try it.
	calls          EndpointCaller
	callsProtected bool
	log            *slog.Logger

	// preview asks `brevis-sql serve` for a destination's first rows. Nil
	// when no service is configured, and then the tab does not exist.
	preview *sqlserve.Client

	// protected says this console asks for a login. See dataTools.
	protected bool

	// gallery serves /_components. See WithGallery.
	gallery bool

	// hub feeds GET /api/events. See WithChangeHub.
	hub *events.Hub
	// scrape is the live volume collector; nil serves "not configured".
	scrape *metrics.Collector
}

// WithPreview points the console at a SQL service, and says whether this
// console has a login.
//
// A SETTER AND NOT AN ARGUMENT, because NewUI already takes eight: this one
// is optional in a way the others are not -- a console with no SQL service is
// a console that works, and every caller would otherwise pass nil.
//
// THE CREDENTIAL ITSELF AND NOT A BOOLEAN, so that nobody can answer this
// question wrongly by writing `true`. The only way to get the tabs is to hand
// over the console's real credential and have it be one; a bool would make
// "we are protected" something a caller could simply assert.
//
// An ARGUMENT and not a second setter, so forgetting it is a compile error
// rather than an open console with a warehouse on it.
// EndpointsReader is what Data → Endpoints reads: what each brevis-sql-api
// was published with. Never its traffic, which is on the API's /metrics.
type EndpointsReader interface {
	List(ctx context.Context) ([]postgres.PublishedEndpoint, error)
	Get(ctx context.Context, api, name string) (postgres.PublishedEndpoint, bool, error)
	Reading(ctx context.Context, target string) ([]postgres.PublishedEndpoint, error)
}

// EndpointCaller runs a published endpoint for Try it.
type EndpointCaller interface {
	Configured(api string) bool
	Call(ctx context.Context, api, path string, params url.Values) (endpointcall.Answer, error)
}

// WithEndpointCalls turns on Try it, for the APIs the caller has an address
// for, AND ONLY ON A CONSOLE THAT ASKS WHO YOU ARE: the console's key is one
// tenant's for every login, and an open console would hand that tenant's
// rows to whoever found the address -- the SQL tabs' rule, for their reason.
func (u *UI) WithEndpointCalls(c EndpointCaller, console auth.Credential) *UI {
	u.calls = c
	u.callsProtected = console.Enabled()
	return u
}

// WithGallery serves /_components: every component the screens are built
// from, with sample data, in one page. For somebody building the console --
// the caller turns it on with BREVIS_ENV=local and nowhere else. Off, the
// route is not registered at all, so it is a 404 rather than a page that
// explains itself.
func (u *UI) WithGallery(on bool) *UI {
	u.gallery = on
	return u
}

// FailingReader is the workflows whose last finished run failed.
type FailingReader interface {
	FailingWorkflows(ctx context.Context) ([]postgres.FailingWorkflow, error)
	// FailureStreak is how many times in a row a workflow failed, for the
	// open run's "why it failed".
	FailureStreak(ctx context.Context, slug string) (int, error)
}

// EventsReader reads the event log: what the installation did, newest first.
type EventsReader interface {
	RecentEvents(ctx context.Context, q postgres.EventQuery) ([]postgres.Event, string, error)
}

// WithEvents turns on the Overview's event log. Without it the tab is empty.
func (u *UI) WithEvents(r EventsReader) *UI {
	u.events = r
	return u
}

// UpdateSource knows the newest Brevis released and whether it is newer than
// the one running.
type UpdateSource interface {
	Latest() (r updates.Release, newer, ok bool)
}

// WithVersion shows the running version in the account menu and, when src
// knows a newer release, an update icon and a notice. A nil src is the version
// alone.
func (u *UI) WithVersion(current string, src UpdateSource) *UI {
	u.version, u.updates = current, src
	return u
}

// WithFailing turns on the list of failing workflows.
func (u *UI) WithFailing(r FailingReader) *UI {
	u.failing = r
	return u
}

// WithEndpoints turns on Data → Endpoints and the "Read by" of a model's
// page.
func (u *UI) WithEndpoints(r EndpointsReader) *UI {
	u.endpoints = r
	return u
}

func (u *UI) WithPreview(c *sqlserve.Client, console auth.Credential) *UI {
	u.preview = c
	u.protected = console.Enabled()
	return u
}

// dataTools says whether a destination may offer its Preview and Query tabs.
//
// THREE CONDITIONS, AND ONLY ONE IS ABOUT SQL.
//
// A console can run with no credential -- it warns at boot, "interface is
// OPEN: anyone can trigger a workflow" -- and the Query tab made that
// sentence incomplete: it is now also anyone can read every table in the
// project, because a query is not confined to the destination it was opened
// from. The tab does not create that hole, it changes what falls through it,
// so a data tool does not outlive the authentication of the screen it sits
// on.
//
// And a destination has to be a RELATION. A bucket and a topic have no
// columns and no rows; a tab on one is a box that can only ever say no,
// which is worse than no tab because it invites somebody to try. A relation
// nobody has written a reader for -- `mysql://` today -- keeps its tabs,
// because there the refusal is a sentence worth reading.
func (u *UI) dataTools(target string) bool {
	return u.protected && u.preview.Configured() && catalog.IsRelation(target)
}

func NewUI(l Leitura, d Definitions, e RunsChart, a Actions, al AlertsReader,
	c CatalogReader, m branding.Brand, log *slog.Logger,
) *UI {
	u := &UI{leitura: l, defs: d, execs: e, actions: a, alerts: al, catalog: c, brand: m, log: log}
	if l != nil {
		// Every page and every open tab's 10 s poll draws the badge.
		u.inFlight = CachedInFlight(l.CountRunsInFlight, 5*time.Second, time.Now)
	}
	return u
}

// Registrar wires the routes into the mux.
func (u *UI) Registrar(mux *http.ServeMux) {
	// The command palette's index: everything the console lists, fetched once
	// per page and filtered in the browser. JSON under /api, like the graph.
	mux.HandleFunc("GET /api/palette", u.palette)
	u.registerEvents(mux)
	// The three routes that are not data and not a page: the log as a download, the
	// manual trigger as a form post (the Load drawer's button; also what a curl
	// calls) and the language choice.
	mux.HandleFunc("GET /runs/{id}/log.txt", u.runLog)
	mux.HandleFunc("POST /preferences/lang", u.chooseLang)
	mux.HandleFunc("POST /workflows/{slug}/trigger", u.disparar)

	// The JSON the graph fetches.
	mux.HandleFunc("GET /api/workflows/{slug}/graph", u.workflowGraph)
	mux.HandleFunc("GET /api/runs/{id}/graph", u.runGraph)

	// Served from the embed, not from disk: the container is distroless and has
	// no web/assets, and the binary has to work from any directory.
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assets.Handler()))

	// The console, last and least specific: "GET /" takes every GET that nothing
	// above claimed (a client-side route), so each engine path wins over it.
	// /app and /app/... are where the console lived before; they redirect.
	mux.Handle("GET /", spa.Handler())
	mux.HandleFunc("GET /app", spa.LegacyRedirect)
	mux.HandleFunc("GET /app/{path...}", spa.LegacyRedirect)
	mux.HandleFunc("GET /api/session", u.session)
	mux.HandleFunc("GET /api/nav/runs", u.navRunsJSON)
	mux.HandleFunc("GET /api/release", u.release)
	mux.HandleFunc("GET /api/config", u.config)
	mux.HandleFunc("GET /api/projects", u.projects)
	mux.HandleFunc("GET /api/messages", u.messages)
	u.registerSQLJSON(mux)
	u.registerWorkflowsAPI(mux)
	u.registerTriggerAPI(mux)
	u.registerRunAPI(mux)
	u.registerIngestAPI(mux)
	u.registerOverviewAndRunsAPI(mux)
	u.registerCatalogAPI(mux)
	u.registerMetricsAPI(mux)
}

// targetCeiling matches the engine's own bound on a target. A legacy label
// is looked up as stored, so `u` is not shape-checked beyond this: an unknown
// value is a 404, not a 400.
const targetCeiling = 512

// targetTools fills in what a destination offers beyond its loads -- the
// endpoints that read it, and whether a SQL service answers for it, for the
// link to the workbench -- for the catalog's drawer and a load's alike.
func (u *UI) targetTools(r *http.Request, v *pages.TargetView, target string) error {
	if u.endpoints != nil {
		readers, err := u.endpoints.Reading(r.Context(), target)
		if err != nil {
			return err
		}
		v.ReadBy = pages.ReadersOf(readers)
	}

	// NO SERVICE, NO QUERY. A link to a workbench that always answers "not
	// configured" is a question nobody can act on.
	v.Tabs = u.dataTools(target)

	return nil
}

// rowsRead asks the newest load's run what its step read (screen-load-v2
// LV2). One read by run id; a failure is a page without the count, not an
// error -- the landing's own numbers are still right.
func (u *UI) rowsRead(ctx context.Context, v pages.LoadView, step string) *int64 {
	if u.execs == nil || len(v.Loads) == 0 {
		return nil
	}
	id, err := uuid.Parse(v.Loads[0].RunID)
	if err != nil {
		return nil
	}
	states, err := u.execs.NodeStates(ctx, id)
	if err != nil {
		u.log.Warn("the load's run stages are unavailable", "run", id, "error", err)
		return nil
	}
	// A MAPPED STEP HAS NO ONE ANSWER. Its node state carries one
	// instance's stages, and the newest load is one instance's landing: the
	// two need not be the same instance, and "2 of 3 read" mixing them would
	// be a number that is wrong in the way that matters most.
	if states[step].Instances > 0 {
		return nil
	}
	return pages.RowsRead(states[step].Stages)
}

// queryRows is how many a query draws, and statementCeiling how long a
// statement may be.
//
// MORE THAN A PREVIEW, because they are different questions: a preview is a
// glance at the shape of a table and a query is somebody's own report. The
// SERVICE has the real ceiling and this cannot raise it; this asks for fewer.
//
// The ceiling matches `serve`'s body limit rather than undercutting it, so a
// statement refused here is refused for the same reason it would be there --
// two different limits would mean a statement that one accepts and the other
// does not, and a refusal nobody can explain.
const (
	queryRows        = 100
	statementCeiling = 64 << 10
)

// paletteItem is one thing the command palette can go to. Label and Hint are a string (a name the data
// owns) or an i18n.Msg (a sentence the app says); the app builds what a search looks in from what it says.
type paletteItem struct {
	// Action, when set, is what the island does instead of following Href.
	Action string `json:"action,omitempty"`
	// Group is the group's id: the island orders and filters by it and says its name.
	Group string `json:"group"`
	Label any    `json:"label"`
	Hint  any    `json:"hint,omitempty"`
	Href  string `json:"href"`
	Icon  string `json:"icon"`
}

func paletteEntry(group string, label, hint any, href, icon string) paletteItem {
	if hint == "" {
		hint = nil
	}
	return paletteItem{Group: group, Label: label, Hint: hint, Href: href, Icon: icon}
}

// palette is the command palette's index: the sections, then the workflows,
// projects, streams, loads and endpoints, each with where it goes.
//
// EVERY SOURCE IS OPTIONAL. A console with no endpoints reader lists none, the
// way its sidebar offers none; one source missing never takes the rest with it.
// A source that FAILS is an error, though: a partial index that looks whole is
// how somebody concludes a workflow does not exist.
//
// NO-STORE. It names everything this login can see, and it is fetched fresh
// with each page that opens the palette.
func (u *UI) palette(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items := []paletteItem{}
	for _, s := range sections(u.preview.Configured()) {
		items = append(items, paletteEntry("Sections", s.Name, s.Group, s.Href, s.Icon))
	}
	// Actions do something here rather than go somewhere. The theme is the
	// console's own; the rest arrive with the screens that own them.
	theme := paletteEntry("Actions", i18n.M("palette.theme"), i18n.M("palette.theme.hint"), "#", "sun")
	theme.Action = "toggle-theme"
	items = append(items, theme)
	for _, lang := range i18n.Langs {
		it := paletteEntry("Actions", i18n.M("palette.lang", "name", i18n.Names[lang]), i18n.Codes[lang], "#", "globe")
		it.Action = "set-lang:" + lang
		items = append(items, it)
	}
	if u.leitura != nil {
		wfs, err := u.leitura.Workflows(ctx)
		if err != nil {
			u.failure(w, r, err)
			return
		}
		for _, wf := range wfs {
			items = append(items, paletteEntry("Workflows", wf.Name, wf.Project, "/workflows/"+url.PathEscape(wf.Slug), "workflow"))
		}
		projects, err := u.leitura.Projects(ctx)
		if err != nil {
			u.failure(w, r, err)
			return
		}
		for _, p := range projects {
			items = append(items, paletteEntry("Projects", p.Name, i18n.MN("loads.summary.workflows", p.Workflows),
				"/workflows?project="+url.QueryEscape(p.Slug), "folder"))
		}
	}
	if u.catalog != nil {
		entries, err := u.catalog.Catalog(ctx)
		if err != nil {
			u.failure(w, r, err)
			return
		}
		// A stream by what it is, POST <path>; a load by step → where.
		// A destination by what it is, the writer as the hint.
		for _, row := range pages.BuildCatalog(entries, pages.DataFilter{}, time.Now()).Rows {
			items = append(items, paletteEntry("Catalog", row.Target, pages.WriterLine(row), pages.TargetHref(row.Target), "database"))
		}
		for _, g := range pages.StreamsOf(entries) {
			for _, s := range g.Streams {
				items = append(items, paletteEntry("Streams", "POST "+s.Path, g.Name, s.Href(), "cloud"))
			}
		}
		for _, t := range pages.LoadsOf(entries, time.Now()) {
			if t.URI == "" {
				continue
			}
			items = append(items, paletteEntry("Loads", t.Step+" → "+t.URI, t.Workflow, t.Href(), "download"))
		}
	}
	if u.endpoints != nil {
		all, err := u.endpoints.List(ctx)
		if err != nil {
			u.failure(w, r, err)
			return
		}
		for _, row := range pages.BuildEndpoints(all, nil, pages.EndpointFilter{}, time.Now()).Rows {
			items = append(items, paletteEntry("Endpoints", row.Path, row.API, row.Href(), "plug"))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(struct {
		Items []paletteItem `json:"items"`
	}{items}); err != nil {
		u.log.Error("writing the palette's index", "error", err)
	}
}

// nextRuns computes each active schedule's next dispatch and returns
// the soonest first.
//
// The computation lives here and not in the database: cron is a domain rule
// (`schedule`), and reimplementing it in SQL would create a second reading of
// the same field -- one that would one day diverge from the one the scheduler
// actually uses.
func nextRuns(schedules []postgres.ScheduleSummary, now time.Time, limite int,
	log *slog.Logger) []pages.NextRun {

	var out []pages.NextRun
	for _, a := range schedules {
		if !a.Active {
			continue
		}
		s := sch.Schedule{WorkflowSlug: a.WorkflowSlug, Cron: a.Cron, Timezone: a.Timezone, Active: true}
		prox, err := s.Next(now)
		if err != nil {
			// An invalid cron in the database must not take the whole dashboard
			// down; the schedule simply does not show up in the list.
			log.Warn("invalid cron while computing the next trigger",
				"workflow", a.WorkflowSlug, "cron", a.Cron, "error", err)
			continue
		}
		out = append(out, pages.NextRun{
			Workflow: a.WorkflowSlug, Cron: a.Cron, Timezone: a.Timezone, When: prox,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.Before(out[j].When) })
	if len(out) > limite {
		out = out[:limite]
	}
	return out
}

// runsList reads the column: the page of runs, how many there are, and the
// counts per state -- the same filter WITHOUT its state, so each choice says
// what its link would show. No reader is an empty column: a process wired
// for the run alone still opens it.
func (u *UI) runsList(ctx context.Context, query postgres.RunFilter) ([]postgres.RunSummary, int, map[string]int, error) {
	if u.leitura == nil {
		return nil, 0, nil, nil
	}
	total, err := u.leitura.CountRuns(ctx, query)
	if err != nil {
		return nil, 0, nil, err
	}
	runs, err := u.leitura.Runs(ctx, query)
	if err != nil {
		return nil, 0, nil, err
	}
	byState := query
	byState.State, byState.States, byState.Limite, byState.Offset = "", nil, 0, 0
	counts, err := u.leitura.CountRunsByState(ctx, byState)
	if err != nil {
		return nil, 0, nil, err
	}
	return runs, total, counts, nil
}

func inFlight(counts map[string]int) (n int) {
	for _, s := range pages.InFlight {
		n += counts[s]
	}
	return
}

// validState refuses any state outside §7's machine.
//
// This is not about SQL -- the value already goes parameterised. It is about the
// screen: `?estado=xpto` would return an empty list with every chip greyed out,
// and the operator would read that as "there are no runs" rather than "that
// filter does not exist".
func validState(s string) string {
	switch s {
	case "queued", "running", "success", "failed", "retrying", "canceled":
		return s
	}
	return ""
}

func page(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// instante accepts the RFC3339 the chart's links emit. An invalid value becomes
// no filter rather than an error: a half-pasted link should not give a 500.
func instant(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// contar is how many workflows each state would show: the same filter with
// that one field swapped, run through filtrar, so a count cannot disagree
// with the list its link opens.
func contar(all []postgres.WorkflowSummary, f pages.Filter, x listFacts) pages.WorkflowCounts {
	c := pages.WorkflowCounts{State: map[string]int{}}
	for _, s := range []string{"", "failing", "running", "paused", "manual"} {
		g := f
		g.State = s
		c.State[s] = len(filtrar(all, g, x))
	}
	return c
}

// validOrder limits sorting to the columns that exist -- without it, `?ordem=;`
// would only produce a list in an unexplained order.
func validOrder(s string) string {
	switch s {
	case "workflow", "schedule", "next", "last":
		return s
	}
	return ""
}

// sortBy applies the chosen column.
//
// Two rules the naive inversion (comparing with the arguments swapped) broke: a
// MISSING value stays last in both directions -- sorting by "last run" must not
// start with the ones that never ran -- and the slug tie-break is always
// ascending, or two equivalent rows swap places on every load.
func sortBy(ws []postgres.WorkflowSummary, f pages.Filter) {
	if f.Sort == "" {
		return
	}
	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i], ws[j]
		temA, temB := hasValue(a, f.Sort), hasValue(b, f.Sort)
		if temA != temB {
			return temA
		}
		if temA {
			if c := compareField(a, b, f.Sort); c != 0 {
				if f.Desc {
					return c > 0
				}
				return c < 0
			}
		}
		return a.Slug < b.Slug
	})
}

func hasValue(w postgres.WorkflowSummary, field string) bool {
	switch field {
	case "schedule":
		return w.Cron != ""
	case "next":
		return w.NextRun != nil
	case "last":
		return w.LastRunAt != nil
	}
	return true
}

func compareField(a, b postgres.WorkflowSummary, field string) int {
	switch field {
	case "schedule":
		return strings.Compare(a.Cron, b.Cron)
	case "next":
		return comparaTempo(a.NextRun, b.NextRun)
	case "last":
		return comparaTempo(a.LastRunAt, b.LastRunAt)
	}
	return strings.Compare(a.Slug, b.Slug)
}

// comparaTempo only receives present values: absence is decided earlier, in
// `temValor`, precisely so it does not depend on the direction.
func comparaTempo(a, b *time.Time) int {
	switch {
	case a.Before(*b):
		return -1
	case a.After(*b):
		return 1
	}
	return 0
}

// recortar returns the requested page. A page past the end comes back empty
// rather than overflowing the slice -- which happens when filtering while on a
// high page.
func recortar(ws []postgres.WorkflowSummary, f pages.Filter) []postgres.WorkflowSummary {
	de := (f.Page - 1) * f.PerPage
	if de >= len(ws) {
		return nil
	}
	ate := de + f.PerPage
	if ate > len(ws) {
		ate = len(ws)
	}
	return ws[de:ate]
}

func proximaDoWorkflow(w postgres.WorkflowSummary, now time.Time) *time.Time {
	if !w.Active || w.Cron == "" {
		return nil
	}
	s := sch.Schedule{WorkflowSlug: w.Slug, Cron: w.Cron, Timezone: w.Timezone, Active: true}
	prox, err := s.Next(now)
	if err != nil {
		return nil
	}
	return &prox
}

// filtrar applies the search bar in memory.
//
// In memory and not in SQL because the workflow list is in the dozens, and
// filtering by LAST state would mean repeating the query's LATERAL inside a
// WHERE. If it ever becomes thousands, this turns into a database predicate.
func filtrar(ws []postgres.WorkflowSummary, f pages.Filter, x listFacts) []postgres.WorkflowSummary {
	search := strings.ToLower(f.Search)
	out := make([]postgres.WorkflowSummary, 0, len(ws))
	for _, w := range ws {
		if search != "" && !strings.Contains(x.haystack(w), search) {
			continue
		}
		if !x.inState(w, f.State) {
			continue
		}
		switch f.Active {
		case "active":
			if !w.Active {
				continue
			}
		case "paused":
			// No schedule is not "paused": it is a workflow that never had a
			// cron, and mixing the two would hide precisely the schedule that
			// was turned off.
			if w.Active || !w.HasSchedule {
				continue
			}
		}
		if !containsAll(w.Tags, f.Tags) {
			continue
		}
		if f.Project != "" && w.Project != f.Project {
			continue
		}
		if f.Lang != "" && !contains(x.langs(w.Slug), f.Lang) {
			continue
		}
		out = append(out, w)
	}
	return out
}

func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// containsAll says whether a workflow carries every chosen tag: two tags
// narrow the list, they do not widen it.
func containsAll(have, want []string) bool {
	for _, t := range want {
		if !contains(have, t) {
			return false
		}
	}
	return true
}

// tagsOf reads the repeated `tag=` into the chosen set, without blanks or
// repeats -- `?tag=a&tag=a` is one tag, and a link built from it says so.
func tagsOf(values []string) []string {
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// tagsDe gathers the tags of ALL workflows, not of the filtered rows: the filter
// bar must not shrink as you filter, or there is no way back.
func tagsDe(ws []postgres.WorkflowSummary) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, w := range ws {
		for _, t := range w.Tags {
			if _, ja := seen[t]; ja {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// runLog is the run's whole log as one text file: every attempt of every
// step, in the order they ran, each under a header that says which it is and
// how it ended. The file to keep, or to send to whoever owns the step.
func (u *UI) runLog(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "not a run id", http.StatusBadRequest)
		return
	}
	current, err := u.execs.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		u.failure(w, r, err)
		return
	}
	logs, err := u.execs.LogsDaRun(r.Context(), id)
	if err != nil {
		u.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+fileSafe(current.WorkflowSlug)+"-"+id.String()[:8]+`.log"`)
	var b strings.Builder
	for i, l := range logs {
		if i > 0 {
			b.WriteString("\n")
		}
		step := l.NodeID
		if l.MapIndex != run.Unmapped {
			step += "[" + strconv.Itoa(l.MapIndex) + "]"
		}
		// Attempts are stored from 0; a reader counts from 1.
		fmt.Fprintf(&b, "== %s · attempt %d · %s", step, l.Attempt+1, l.Status)
		if l.ExitCode != nil {
			fmt.Fprintf(&b, " · exit %d", *l.ExitCode)
		}
		b.WriteString("\n")
		if text := pages.AttemptText(l); text != "" {
			b.WriteString(text + "\n")
		}
	}
	_, _ = io.WriteString(w, b.String())
}

// fileSafe keeps a file name to letters, digits, '-', '_' and '.': a slug is
// the author's text, and a quote or a newline would break the header.
func fileSafe(s string) string {
	return strings.Map(func(c rune) rune {
		if c < utf8.RuneSelf && (unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("-_.", c)) {
			return c
		}
		return '_'
	}, s)
}

// runPage reads what the run page and its live refresh are both drawn from.
// ONE helper for both, so they cannot disagree about what a run is.
//
// A missing log or alert does not block the screen: the rest of the page
// stays useful, and a freshly queued run legitimately has no steps yet. An
// alert nobody can see is indistinguishable from one never sent, which is
// what that block exists to fix -- a blank page would be worse at it than a
// page missing one block.
func (u *UI) runPage(ctx context.Context, current run.Run) pages.RunPage {
	p := pages.RunPage{Run: current, Wait: u.waitOf(ctx, current)}
	var err error
	// What the run ran with decides whether it is one step, not what the
	// workflow says now. Its states are read only then: a multi-step run's
	// are the graph's, which the island fetches on its own cadence.
	var def wf.Workflow
	if json.Unmarshal(current.Definition, &def) == nil && oneStep(def) {
		states, err := u.execs.NodeStates(ctx, current.ID)
		if err != nil {
			u.log.Warn("the run's step states are unavailable", "run", current.ID, "error", err)
		}
		p.Step = stepLineOf(def, states)
	}
	if p.Logs, err = u.execs.LogsDaRun(ctx, current.ID); err != nil {
		u.log.Warn("the run's logs are unavailable", "run", current.ID, "error", err)
	}
	if u.alerts != nil {
		if p.Raised, err = u.alerts.ForRun(ctx, current.ID); err != nil {
			u.log.Warn("the run's alerts are unavailable", "run", current.ID, "error", err)
		}
	}
	// The CURRENT workflow, for Rerun and the head's logos: what the trigger
	// resolves against. Read here and not only for the page, because the
	// refresh that turns a running run into a finished one draws its Rerun.
	// A workflow that is gone is a head without the button, not a refusal --
	// its runs stay readable.
	if u.defs != nil {
		if def, err := u.defs.Definition(ctx, current.WorkflowSlug); err == nil {
			p.Workflow = &def
			p.Logos = workflowLogos(def)
		}
	}
	u.whyItFailed(ctx, &p)
	return p
}

// whyItFailed reads why a failed or retrying run failed, from the logs the
// page already has, and how many times in a row its workflow has failed. A
// streak that cannot be read is a head without that line.
func (u *UI) whyItFailed(ctx context.Context, p *pages.RunPage) {
	if p.Run.Status != run.StatusFailed && p.Run.Status != run.StatusRetrying {
		return
	}
	p.Cause = causeOf(p.Logs, p.Run.Err)
	if p.Run.Status == run.StatusFailed && u.failing != nil {
		n, err := u.failing.FailureStreak(ctx, p.Run.WorkflowSlug)
		if err != nil {
			u.log.Warn("the workflow's failure streak is unavailable", "workflow", p.Run.WorkflowSlug, "error", err)
		}
		p.Streak = n
	}
}

// waitOf asks what holds a queued run, on the terms of the logs and alerts: a
// failed question is a page without the line, not a page that refuses (#95).
// Only a queued run is asked -- nothing else is in the queue to be held.
func (u *UI) waitOf(ctx context.Context, current run.Run) *postgres.Wait {
	if current.Status != run.StatusQueued {
		return nil
	}
	wait, err := u.execs.WaitingOn(ctx, current.ID)
	if err != nil {
		u.log.Warn("could not learn what holds the run", "run", current.ID, "error", err)
		return nil
	}
	return wait
}

// paramsFromForm reads the `param.`-prefixed fields of the trigger form.
//
// Values are joined with a comma, because that is how a `list|<type>` param
// travels everywhere else -- and it is what makes a checkbox group work without
// a line of JavaScript: the browser sends one `param.layers` per box ticked,
// and an unticked box sends nothing at all.
//
// Each list also carries a hidden empty value, which is what makes "none of
// them" different from "the operator did not touch this". With every box
// unticked and no hidden field, the key would be absent from the form and the
// resolver would fall back to the default -- quietly running with the values
// the operator had just cleared.
func paramsFromForm(form url.Values) map[string]string {
	params := map[string]string{}
	for key, values := range form {
		name, ok := strings.CutPrefix(key, "param.")
		if !ok {
			continue
		}
		nonEmpty := make([]string, 0, len(values))
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				nonEmpty = append(nonEmpty, v)
			}
		}
		params[name] = strings.Join(nonEmpty, ",")
	}
	return params
}

func (u *UI) disparar(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// The params come from the form, prefixed with `param.` so they do not
	// collide with future fields of the form itself.
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	params := paramsFromForm(r.PostForm)
	slot, err := u.slotFromForm(r.Context(), slug, slotField(r.PostForm))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := u.actions.Disparar(r.Context(), slug, time.Now(), params, slot)
	if err != nil {
		// An invalid param is an INPUT error, not the server's: a 500 here would
		// send the operator looking for a defect in the platform rather than in
		// the value they typed.
		u.log.Warn("trigger refused", "workflow", slug, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	next := "/runs/" + id.String()
	if id == uuid.Nil {
		// The idempotency key collided: two clicks in the same second become
		// one run. Going back is the right behaviour -- there is no new run to
		// go to.
		u.log.Info("trigger ignored by idempotency", "workflow", slug)
		next = backTo(r)
	} else {
		u.log.Info("manual run created", "workflow", slug, "run", id)
	}
	// The Run dialog posts with its script, to stay open on a refusal: it is
	// told where to go instead of being redirected there.
	if fragment(r) == "trigger" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct{ Location string }{next})
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// slotField is the slot the form asks for: none when it says "now", the time
// typed beside the list when it says "other", and the `slot` field otherwise
// -- which is what a form with no `when`, or a curl, sends.
func slotField(form url.Values) string {
	if form.Get("when") == "now" {
		return ""
	}
	slot := strings.TrimSpace(form.Get("slot"))
	if slot == "other" {
		return strings.TrimSpace(form.Get("slot_at"))
	}
	return slot
}

// slotFromForm reads the `slot` field: an RFC3339 instant, which says its own
// offset, or the browser's datetime-local, which says none and is read in the
// schedule's zone -- the zone its cron is written in. Empty is no slot.
// Whether it is a slot of the schedule is the scheduler's to say.
func (u *UI) slotFromForm(ctx context.Context, slug, v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	loc := time.UTC
	if u.leitura != nil {
		schedules, err := u.leitura.Schedules(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range schedules {
			if a.WorkflowSlug == slug && a.Timezone != "" {
				if l, err := time.LoadLocation(a.Timezone); err == nil {
					loc = l
				}
			}
		}
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return &t, nil
		}
	}
	return nil, &slotParseError{Value: v}
}

// backTo is the page the request came from, on this site, or the Workflows
// list.
func backTo(r *http.Request) string {
	target := r.Referer()
	// It only accepts a destination on this site: an external Referer would turn
	// the redirect into an open-redirect vector.
	if target == "" || !strings.HasPrefix(target, "/") {
		if u := parseMesmoHost(r); u != "" {
			target = u
		} else {
			target = "/workflows"
		}
	}
	return target
}

// parseMesmoHost accepts the Referer only when it points at this same host.
func parseMesmoHost(r *http.Request) string {
	ref := r.Referer()
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host != r.Host {
		return ""
	}
	path := u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path
}

// chooseLang remembers the reader's language and goes back to the page they
// were on. A plain form POST, so it works with no script; `next` goes through
// auth.Target, the login's own guard against an open redirect.
func (u *UI) chooseLang(w http.ResponseWriter, r *http.Request) {
	if !i18n.Remember(w, r.FormValue("lang")) {
		http.Error(w, "unknown language", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, auth.Target(r.FormValue(auth.NextParam)), http.StatusSeeOther)
}

// fragment is the part of a page a request asks for alone, from the
// X-Brevis-Fragment header; empty asks for the whole page.
//
// A HEADER AND NOT A ROUTE. The fragment is the same handler's answer in
// another shape -- the same refusals, the same limits -- and a second route
// would be a second place for those to drift. A handler that has no fragment
// ignores the header and draws its page.
func fragment(r *http.Request) string {
	return r.Header.Get("X-Brevis-Fragment")
}

func (u *UI) failure(w http.ResponseWriter, r *http.Request, err error) {
	u.log.Error("querying the ui's data", "path", r.URL.Path, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
