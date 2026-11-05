# Observability

Brevis exposes Prometheus metrics on a port of its own, from **both** processes.
It has no traces yet, and [why not](#what-is-deliberately-absent) is at the
bottom.

```
GET http://<pod>:9090/metrics
```

## Two processes, two endpoints

This is the part most setups get wrong by scraping only the API.

| | serves | reports |
|---|---|---|
| `brevis serve` | the UI and the HTTP API | queue depth |
| `brevis scheduler` | nothing over HTTP | queue depth, claim latency, slots, orphans, runs, steps |

**The interesting numbers live in the scheduler.** It is the process that claims
work and runs steps; the API only serves screens. A dashboard built on the API's
scrape alone is a dashboard of an idle process.

## Why a separate port

`BREVIS_METRICS_ADDR` defaults to `:9090` and is deliberately not
`BREVIS_HTTP_ADDR`.

The HTTP port is the one behind the Ingress and behind `auth.Gate`. Putting
`/metrics` there leaves two options and both are wrong: inside the gate, no
scraper can reach it, because the session is a cookie set by a login form;
outside it, every workflow and step name is published to whoever finds the path.

On its own port it is never in the Ingress. Keep it that way — the port belongs
in a `ServiceMonitor` or a scrape annotation, not in an `Ingress` rule.

Set `BREVIS_METRICS_ADDR=""` to serve nothing. An **empty** value means off,
which is different from unset — unset gets the default.

## The live volume on the console's screens

The Streams and Endpoints screens (list and detail) draw what was sent to the
streams and what the endpoints served, per minute, as it happens. The console has
no time series store: it **scrapes the `/metrics` of the gateways and the SQL
APIs you name** and keeps per-minute buckets in memory.

| variable | |
|---|---|
| `BREVIS_GATEWAY_METRICS_URLS` | `events_gateway=http://gateway:9090/metrics` — the gateway's `name:`, then the address of **its metrics listener** (`metrics.addr`, `:9090` by default; on the compose demo, `${BREVIS_GATEWAY_METRICS_PORT:-9091}`), never the ingest port. Repeat a name for replicas |
| `BREVIS_ENDPOINTS_METRICS_URLS` | `example-api=http://endpoints:9090/metrics` — the API's name as in `BREVIS_ENDPOINTS_URLS`, then its `metrics.addr`. Always its own variable: the metrics listener is never the API's port |
| `BREVIS_METRICS_SCRAPE_INTERVAL` | `5s`; `1s` to `1m`. Each target is read at a random offset, +-10%, with a timeout of the interval (5 s at most), at most 8 MiB, no redirect and no proxy |

Targets come **only** from these variables, never from a request, so nobody who
can log in can aim the console at an address. An address with a credential in it
is refused at start, and an address is never logged or returned: errors name the
variable, the entry and the gateway or API.

Which series feed which number:

| screen | number | series |
|---|---|---|
| stream | received, rejected, dropped, dead letter per minute | `brevis_gateway_events_received_total`, `_rejected_total`, `_dropped_total`, `brevis_gateway_dead_letter_records_total` |
| stream | delivered batches | `brevis_gateway_batches_total{outcome="delivered"}` — **batches**, not events: the gateway has no per-event delivery counter unless `BREVIS_INGESTION_METRICS` is on |
| stream | buffered, queued now | `brevis_gateway_buffer_records`, `brevis_gateway_queue_batches` |
| stream | delivery p50 / p95 | `brevis_gateway_delivery_seconds` buckets |
| endpoint | requests by outcome | `brevis_sql_api_requests_total{status}`: 2xx/3xx ok, 429 rate limited, other 4xx client errors, 5xx server errors. A request that reached no endpoint (`endpoint="-"`) belongs to none |
| endpoint | cache hit ratio | `brevis_sql_api_cache_requests_total{result}` — hits over hits and misses; an endpoint without a cache has no ratio, shown as "—" |
| endpoint | hook failures | `brevis_sql_api_hook_total{outcome!="ok"}` |
| endpoint | p50 / p95 | `brevis_sql_api_request_seconds` buckets |

A stream is matched to its series by the `name` of its published configuration
(the gateway labels by name, the console keys a stream by gateway and path). The
percentiles are interpolated inside histogram buckets: an estimate, not a
measurement.

**A stream's health** is derived from those series, on the server, by one pure
function (`internal/metrics/health.go`, with every threshold a named constant and
the reasoning beside it), and rides on `health` of the Streams list and detail
JSON. Honest or nothing: a state is claimed only from numbers that were measured.

| state | when | reason keys |
|---|---|---|
| `unknown` | no source for the stream's gateway, no good scrape in 3 intervals, the first scrape (a baseline), or the gateway never reported the stream | `source_not_configured`, `source_unreachable`, `waiting`, `not_in_source`, `not_reported` (no `name`), `destination_unnamed` |
| `quiet` | the source is healthy and the last 5 minutes had no events, deliveries or refusals; `since` is the last minute that had some | `no_traffic`, `no_traffic_seen` (none in the ring) |
| `healthy` | traffic and nothing below | `delivering` |
| `degraded` | any dead-lettered record or buried batch; at least 1% of offered events rejected; any refused-because-full request; at least a quarter of delivery attempts retried; delivery p95 over 30 s; some replicas silent | `dead_lettered`, `rejected_some`, `saturated`, `retrying`, `slow_delivery`, `replicas_partial` |
| `failing` | half or more of the window's events rejected (20+ offered) or dead-lettered (20+ received), or of its batches buried (2+), in at least 2 minutes that are each a majority on their own; or the buffer refused requests in 4 of the 5 minutes | `rejected_most`, `dead_letter_most`, `delivery_failing`, `saturating` |

A stream judged `failing` or `degraded` by a window is never shown healthier
until the last 4 raw minutes (the window less one) were all better: the state
worsens at once and improves late, so a noisy stream does not flap; `recovering`
says a state is being held. Events a hook drops **on purpose**
(`events_dropped_total`) are filtering and never count against health, and
`unauthorized` requests (a wrong key, a scan) are not the stream's fault.
Requests refused as malformed, empty or too large never became events: they count
with the rejected ones, as a floor. With some replicas silent a clean result is
not claimed: the state is `degraded` / `replicas_partial`.

**The honest limits**

- **History is the console's memory.** The last three hours are kept per minute;
  a restart of the console starts again, and the screens say "counted since
  <time>". For longer, query your Prometheus, which scrapes the same endpoints.
  (Back-filling from `BREVIS_PROMETHEUS_URL` is not implemented.)
- **Rates are counter deltas.** The first scrape of a target is only a baseline.
  A counter that goes down, or a changed `process_start_time_seconds`, is a
  restart: the new value is the increase. A series that appears is born at zero.
  An interval longer than three scrapes (a target that was down) is **not**
  bridged: those minutes show as not observed instead of one invented spike.
- **An event is counted in the minute of the scrape that saw it**, up to one
  interval after the minute it happened in. The current minute is partial and is
  drawn lighter.
- **Replicas are summed**, each with its own baseline, so one restarting does
  not disturb the others. If some do not answer the numbers are a floor, and the
  screen says how many answer.
- **Bounded memory.** Per target: 20,000 series per scrape, 1,000 streams or
  endpoints, 64 histogram bounds each, 256 bytes per label value; anything past
  that is dropped and counted ("series left out" on screen), never silent.
- **Real time** is the SSE topics `metrics:streams` and `metrics:endpoints`, which
  change once per finished scrape and are carried only while a screen that draws
  them is open.

## What is measured

### The queue and the scheduler

Where the pain actually is, and none of it was visible before.

| metric | type | |
|---|---|---|
| `brevis_queue_depth{state}` | gauge | `pending` vs `claimed`. The first question when things feel slow |
| `brevis_claim_latency_seconds` | histogram | enqueued to claimed. The scheduler's own SLA |
| `brevis_slots_in_use` / `brevis_slots_limit` | gauge | the concurrency ceiling, otherwise invisible until it bites |
| `brevis_orphans_recovered_total` | counter | a worker died. Until now this was a log line and nothing else |

`brevis_queue_depth` is read from the table at scrape time, not counted locally.
The queue is shared: several dispatchers write to it, and a local counter would
report one replica's opinion of a number that belongs to the queue.

### Runs and steps

| metric | type | labels | |
|---|---|---|---|
| `brevis_run_total` | counter | `workflow`, `status`, `trigger` | one per run, not one per attempt |
| `brevis_run_duration_seconds` | histogram | `workflow`, `status` | from the run's CREATION, so queue time is included |
| `brevis_step_duration_seconds` | histogram | `workflow`, `step`, `status` | one per ATTEMPT |
| `brevis_step_attempts_total` | counter | `workflow`, `step` | flapping shows here and nowhere else |

The two "per run" versus "per attempt" choices are the useful ones:

- **A run that fails twice and then succeeds is one success.** The dispatcher
  passes through the same code three times, and counting each pass would report
  three runs where the operator saw one.
- **A step that fails twice and then succeeds is three attempts**, two of them
  labelled `status="failed"`. That is the only place flapping is visible — the
  run is green, the screen is green, and the step has been failing every night
  for a month.

### The cardinality rule

**`run_id` is never a label**, and a test asserts no series carries anything
shaped like a UUID. One unbounded label is how a metrics backend becomes the
most expensive part of an installation.

`workflow` and `step` are bounded by what has been published, which is a real
bound. If you add a metric, keep it that way.

## Scraping it

The endpoint speaks the Prometheus text exposition format, which an
OpenTelemetry Collector scrapes natively — "OTLP standard" and "scrapeable" were
never in tension.

```yaml
# Pod annotations, for a Prometheus that discovers by annotation.
metadata:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
```

A failed scrape is a **500**, not an empty 200. If the queue depth cannot be
read because Postgres is unreachable, the target goes down rather than reporting
a queue that quietly looks calm.

## For a fetcher: `sdk.Meter`

A pipeline's own numbers go somewhere else, and by a different route.

```go
import (
    "brevis.sh/brevis/sdk"
    "brevis.sh/brevis/sdk/metrics/otelmeter"
)

m, err := otelmeter.New(ctx, otelmeter.Options{Insecure: true})
if err != nil {
    return err
}
defer func() { _ = m.Close(context.Background()) }()

sdk.Run(sdk.Pipeline{Meter: m, Source: /* … */})
```

**Setting a `Meter` requires writing no counters.** Records, rows, pages, HTTP
attempts, bytes and the extract and load durations are all numbers the SDK
already tracked; the `Meter` only decides where else they go. Your own calls are
for what only your pipeline knows:

```go
m.Counter("vendor_rejected_total", 12, sdk.A("reason", "missing_cnpj"))
```

### Push here, pull there, and why that is not an inconsistency

The engine's processes are long-lived, so a collector scrapes them. A fetcher is
a pod that lives ninety seconds: by the time anything discovers it and scrapes
it, it is gone. Short-lived processes have to push.

Which makes **`Close` the most important call in that example**. The periodic
reader's interval is tens of seconds and a fetcher that runs for twelve exits
before the first tick — every number it recorded dies with the process unless
the flush happens on the way out. Use a fresh context for it: the run's is
usually already cancelled by then, and passing it aborts the flush at the exact
moment it matters.

The endpoint comes from `OTEL_EXPORTER_OTLP_ENDPOINT`, the variable every
OpenTelemetry tool already honours, rather than a `BREVIS_` one invented beside
it.

### Name a duration `*_seconds`

The convention is the contract. OpenTelemetry's default histogram boundaries are
`[0, 5, 10, 25 … 10000]`, tuned for milliseconds — a four-minute extract lands in
the first bucket under them, and the chart looks plausible and says nothing.
`otelmeter` gives second-scale boundaries to any histogram whose name ends in
`_seconds`, and leaves everything else alone.

### The interface costs nothing; the implementation is a separate module

`sdk.Meter` is two methods in the root package and no dependencies —
`pruning-check.sh` builds the same consumer with and without one and fails if
the dependency sets differ.

`sdk/metrics/otelmeter` has its own `go.mod`, and that is not tidiness. Putting
the OTLP exporter in `sdk/go.mod` made `go mod tidy` resolve the whole graph
upward — BigQuery 1.50 → 1.72 — and a consumer of `sdk/to/bigquery` went from
460 packages to 736 **without importing anything new**. Package-level pruning
does not protect anybody from a module-level version bump.

## What is deliberately absent

**Infrastructure — CPU, memory, pod restarts.** The engine does not have these
numbers, and the honest source is the cluster's own metrics, which a collector
already scrapes. Re-collecting them here would be a worse `kube-state-metrics`.

**Metrics on the task pods.** They are short-lived and scraping them is a race.
Their numbers arrive through the SDK's `@brevis:` stage protocol, which already
works — see [`SDK_ARCHITECTURE.md`](SDK_ARCHITECTURE.md).

**Traces.** They are the more valuable half of OpenTelemetry for debugging a
distributed run, and a bigger design: context propagation across a pod boundary,
sampling, and a backend to send them to. Metrics first, because the questions
above have no answer today.

**An OTLP push exporter.** It carries protobuf and gRPC, which is a larger
dependency than everything else in this feature put together. An installation
that needs push runs a Collector beside the engine and lets it scrape — which is
what a Collector is for.

## For contributors

The exposition format is written by hand in
`internal/observability/metrics/exposition.go` rather than imported, and the
measurement is the reason. Counted as unions with the engine's own package set,
against a ceiling of 330:

| the engine would compile | packages | |
|---|---|---|
| today | **299** | under the ceiling |
| `+ otel/sdk/metric`, manual reader | **341** | over |
| `+ exporters/prometheus` as well | **388** | far over |

Any metrics library breaks the ceiling, so the OTel SDK's 42 packages are paid
for deliberately. The exporter's further 49 are not: **29 of them are
`google.golang.org/protobuf`**, pulled in because `client_golang` registers
through the generated `client_model`. The engine contains no protobuf otherwise,
and dragging it in to render forty lines of `name{label="value"} 42` — a format
unchanged in a decade — is exactly the transitive drag
`.github/scripts/engine-weight.sh` exists to catch. That script holds the
decision down.

Owning the exposition buys two things besides the weight: the output is exactly
what this document describes, with none of the official exporter's
`otel_scope_name` and `target_info` cardinality; and `/metrics` becomes testable
as bytes, which is worth more than a test asserting that a scrape succeeded.

Two things in that file are easy to get wrong, and both have a test that fails
when the code is reverted:

- OpenTelemetry counts a histogram's buckets **per bucket**; Prometheus reads
  them as **running totals**;
- `le` holds a number inside a string, so sorting the output as text puts
  `le="+Inf"` first and `le="5"` after `le="3600"` — which Prometheus's own
  parser rejects.

That parser runs in the tests. It is a test-only dependency, so protobuf never
reaches the binary, and the format is checked by the reference implementation
rather than by belief.
