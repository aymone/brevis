import { expect, isMetricsRead, mockStreamsList, test } from "./support";
import type { Page, Route } from "@playwright/test";

// The live volume of streams and endpoints, in every state the screens can be in. The sources are mocked: the
// demo stack has no traffic, and "waiting", "unreachable" and "not configured" cannot be summoned on demand.

const MIN = 30;
const ago = (s: number) => new Date(Date.now() - s * 1000).toISOString();
const wave = (n: number, f: (i: number) => number | null) => Array.from({ length: n }, (_, i) => f(i));
const owner = (o: Record<string, unknown> = {}) => ({ name: "events_gateway", state: "ok", replicas: 1, reachable: 1, lastSuccess: ago(3), ...o });

function entity(n: number, kind: "stream" | "endpoint", over: Record<string, unknown> = {}) {
  const values: Record<string, (number | null)[]> =
    kind === "stream"
      ? { received: wave(n, (i) => 40 + (i % 7) * 9), rejected: wave(n, (i) => (i % 5 === 0 ? 3 : 0)), dropped: wave(n, () => 0), deadLetter: wave(n, () => 0), delivered: wave(n, () => 12) }
      : { ok: wave(n, (i) => 30 + (i % 5)), rateLimited: wave(n, () => 0), clientError: wave(n, (i) => (i % 9 === 0 ? 2 : 0)), serverError: wave(n, () => 0), cacheHit: wave(n, () => 20), cacheMiss: wave(n, () => 10), cacheOther: wave(n, () => 0), hookFailed: wave(n, () => 0) };
  values[Object.keys(values)[0]!]![0] = null; // one minute nobody watched
  const sum = (k: string) => (values[k] ?? []).reduce<number>((a, v) => a + (v ?? 0), 0);
  return {
    owner: "", name: "", present: true, values, covered: wave(n, (i) => (i === 0 ? 0 : 60)),
    ratePerMin: Object.fromEntries(Object.keys(values).map((k) => [k, values[k]![n - 1]! * 1.0])),
    lastHour: Object.fromEntries(Object.keys(values).map((k) => [k, sum(k)])), observedSec: 3540,
    gauges: kind === "stream" ? { buffered: 14, queued: 1 } : undefined,
    latencyP50Sec: wave(n, (i) => (i === 0 ? null : 0.04 + (i % 4) * 0.01)), latencyP95Sec: wave(n, (i) => (i === 0 ? null : 0.2 + (i % 3) * 0.05)),
    latency: { p50Sec: 0.05, p95Sec: 0.25, samples: 400, windowMinutes: 5 }, ...over,
  };
}

type State = "ready" | "not_configured" | "waiting" | "unreachable" | "partial" | "quiet" | "absent";
function snapshot(kind: "stream" | "endpoint", n: number, state: State) {
  const o = kind === "stream" ? "events_gateway" : "example-api";
  const configured = state !== "not_configured";
  const owners = !configured
    ? []
    : [owner({ name: o, ...(state === "waiting" ? { state: "waiting", lastSuccess: null } : state === "unreachable" ? { state: "unreachable", reachable: 0, error: "timeout", lastSuccess: ago(190) } : state === "partial" ? { state: "partial", replicas: 2, reachable: 1, dropped: 7 } : {}) })];
  const zero = state === "quiet";
  const ent = (over: Record<string, unknown> = {}) => {
    const e: Record<string, any> = entity(n, kind, over);
    if (zero) {
      for (const k of Object.keys(e.values)) e.values[k] = e.values[k]!.map((v: number | null) => (v === null ? null : 0));
      e.ratePerMin = Object.fromEntries(Object.keys(e.values).map((k) => [k, 0]));
      e.lastHour = Object.fromEntries(Object.keys(e.values).map((k) => [k, 0]));
      e.latency = { p50Sec: null, p95Sec: null, samples: 0, windowMinutes: 5 };
    }
    return e;
  };
  const entities = kind === "stream"
    ? [{ ...ent({ owner: o, name: "clicks", present: state !== "absent" }), gateway: o, path: "/v1/clicks" }, { ...ent({ owner: o, name: "orders" }), gateway: o, path: "/v1/orders" }]
    : [{ ...ent({ owner: o, name: "orders", present: state !== "absent" }), api: o }];
  return {
    configured, since: ago(900), intervalSec: 5, window: { start: Math.floor(Date.now() / 60000) * 60 - (n - 1) * 60, minutes: n }, owners,
    total: ent(), entities: configured ? entities : [],
  };
}

async function mockMetrics(page: Page, state: State, hits: string[] = []) {
  await mockStreamsList(page);
  const fix = (kind: "stream" | "endpoint") => (r: Route) => {
    const u = new URL(r.request().url());
    hits.push(u.pathname);
    const n = Number(u.searchParams.get("minutes") ?? MIN);
    const s = snapshot(kind, n, state);
    if (u.pathname.split("/").length > 5 || /\/endpoints\/[^/]+\/[^/]+\/metrics/.test(u.pathname)) s.entities = s.entities.slice(0, 1);
    return r.fulfill({ json: s });
  };
  await page.route("**/api/ingest/metrics/streams*", fix("stream"));
  await page.route("**/api/ingest/metrics/streams/**", fix("stream"));
  await page.route("**/api/endpoints/metrics*", fix("endpoint"));
  await page.route(/\/api\/endpoints\/[^/]+\/[^/]+\/metrics/, fix("endpoint"));
}


// The Endpoints list with one published endpoint: the band and the rows only exist when there is
// something listed, and a clean database (CI, a fresh checkout) has no published endpoint.
async function mockEndpointsList(page: Page) {
  await page.route(/\/api\/endpoints(\?.*)?$/, (r) =>
    r.fulfill({
      json: {
        now: new Date().toISOString(),
        total: 1,
        apis: 1,
        fresh: 1,
        stale: 0,
        caches: [],
        connections: ["warehouse"],
        apiNames: ["example-api"],
        rows: [
          {
            api: "example-api",
            name: "orders",
            path: "/v1/orders",
            keyed: true,
            about: "Orders per day.",
            freshness: "fresh",
            reads: ["sales.orders"],
            rebuilt: null,
            ttl: "6h",
            params: 1,
            required: 0,
            publishedAt: new Date().toISOString(),
          },
        ],
      },
    }),
  );
}

const STREAMS = "/ingest/streams";
const STREAM = "/ingest/streams/events_gateway/v1/clicks";
const ENDPOINTS = "/data/endpoints";
const ENDPOINT = "/data/endpoints/example-api/orders";

test.describe("streams", () => {
  test("the list shows the band and a rate on each row it has a source for", async ({ page }) => {
    await mockMetrics(page, "ready");
    await page.goto(STREAMS);
    const band = page.locator(".mx-strip");
    await expect(band).toBeVisible();
    await expect(band.locator(".mx-big")).toContainText("/min");
    await expect(band.getByRole("img")).toHaveAttribute("aria-label", /Now:/);
    await expect(band.locator(".mx-note")).toContainText("counted since");
    await expect(page.locator("#stream-rows .mx-row").first()).toBeVisible();
  });

  test("not configured names the variable to set and offers it to copy", async ({ page }) => {
    await mockMetrics(page, "not_configured");
    await page.goto(STREAMS);
    const band = page.locator(".mx-strip");
    await expect(band).toContainText("No metrics source configured");
    await expect(band.locator("code")).toContainText("BREVIS_GATEWAY_METRICS_URLS=");
    await expect(band.getByRole("button", { name: "Copy" })).toBeVisible();
    await expect(band.locator(".mx-big")).toHaveCount(0); // no invented number
    await expect(page.locator("#stream-rows .mx-off").first()).toHaveText("—");
  });

  test("waiting and unreachable say so, with when the source last answered", async ({ page }) => {
    await mockMetrics(page, "waiting");
    await page.goto(STREAMS);
    await expect(page.locator(".mx-strip")).toContainText("Waiting for the first scrape");
    await expect(page.locator(".mx-strip .mx-big")).toHaveCount(0);
    await mockMetrics(page, "unreachable");
    await page.goto(STREAMS);
    await expect(page.locator(".mx-strip")).toContainText("Metrics source unreachable");
    await expect(page.locator(".mx-strip")).toContainText("last answered");
    await expect(page.locator(".mx-strip")).toContainText("timed out");
  });

  test("the detail has a chart that is also a table, the figures, and no layout shift between states", async ({ page }) => {
    await mockMetrics(page, "ready");
    await page.goto(STREAM);
    const card = page.locator(".mx-panel");
    await expect(card.locator(".mx-fig").first()).toBeVisible();
    await expect(card.getByRole("img").first()).toHaveAttribute("aria-label", /last 60 minutes/);
    await expect(card.getByRole("table")).toHaveCount(2); // volume and latency, both for screen readers
    await expect(card.getByRole("table").first().locator("tbody tr")).toHaveCount(10);
    const height = (await card.boundingBox())!.height;
    for (const state of ["not_configured", "waiting", "unreachable", "quiet", "absent"] as const) {
      await page.unrouteAll({ behavior: "ignoreErrors" });
      await mockMetrics(page, state);
      await page.goto(STREAM);
      await expect(card.locator(".mx-fig").first()).toBeVisible();
      await page.waitForTimeout(300);
      expect({ state, height: (await card.boundingBox())!.height }).toEqual({ state, height });
    }
    await page.unrouteAll({ behavior: "ignoreErrors" });
    await mockMetrics(page, "quiet");
    await page.goto(STREAM);
    await expect(card).toContainText("No traffic in the last hour");
  });
});

test.describe("endpoints", () => {
  test("list: the band has requests, error rate and cache hits; rows a rate", async ({ page }) => {
    await mockMetrics(page, "ready");
    await mockEndpointsList(page);
    await page.goto(ENDPOINTS);
    const band = page.locator(".mx-strip");
    await expect(band.locator(".mx-big")).toContainText("requests/min");
    await expect(band.locator(".mx-cells")).toContainText("%");
    await expect(page.locator("#endpoint-rows .mx-row").first()).toBeVisible();
  });

  test("list: not configured names its own variable", async ({ page }) => {
    await mockMetrics(page, "not_configured");
    await mockEndpointsList(page);
    await page.goto(ENDPOINTS);
    await expect(page.locator(".mx-strip code")).toContainText("BREVIS_ENDPOINTS_METRICS_URLS=");
  });

  test("detail: outcomes stacked, percentiles, hit ratio; partial replicas and dropped series are said", async ({ page }) => {
    await mockMetrics(page, "partial");
    await page.goto(ENDPOINT);
    const card = page.locator(".mx-panel");
    await expect(card.getByRole("img").first()).toHaveAttribute("aria-label", /by outcome/);
    await expect(card.locator(".mx-legend").first()).toContainText("Rate limited");
    await expect(card).toContainText("Cache hits, last hour");
    await expect(card.locator(".mx-note")).toContainText("1 of 2 replicas answer");
    await expect(card.locator(".mx-note")).toContainText("7 series left out");
  });
});

test("no horizontal scroll at 390px on any of the four screens", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
  for (const path of [STREAMS, STREAM, ENDPOINTS, ENDPOINT]) {
    const page = await ctx.newPage();
    await mockMetrics(page, "ready");
    await mockEndpointsList(page);
    await page.goto(path);
    await expect(page.locator(".mx-strip, .mx-panel").first()).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth), path).toBeLessThanOrEqual(0);
    await page.close();
  }
  await ctx.close();
});

test("a screen asks for the metrics once, and the event stream carries the topic only while one is open", async ({ page }) => {
  const hits: string[] = [];
  const streams: string[] = [];
  await mockMetrics(page, "ready", hits);
  page.on("request", (r) => {
    if (new URL(r.url()).pathname === "/api/events") streams.push(new URL(r.url()).searchParams.get("topics") ?? "");
  });
  await page.goto(STREAMS);
  await expect(page.locator(".mx-strip .mx-big")).toBeVisible();
  // One request however many rows draw their rate: the key is shared.
  await page.waitForTimeout(700);
  expect(hits.filter((h) => isMetricsRead(h))).toHaveLength(1);
  // After that, at most one per scrape (5 s) from the events the real console sends: never a poll of our own.
  await page.waitForTimeout(6000);
  expect(hits.length).toBeLessThanOrEqual(3);
  await expect.poll(() => streams.some((t) => t.includes("metrics:streams"))).toBe(true);
  // A screen with no metrics does not keep asking: the topic leaves the connection.
  const before = streams.length;
  await page.getByRole("link", { name: /Workflows/ }).first().click();
  await expect.poll(() => streams.length, { timeout: 4000 }).toBeGreaterThan(before);
  expect(streams.at(-1)).not.toContain("metrics:");
});

test("a hidden tab does not read the metrics", async ({ page }) => {
  const hits: string[] = [];
  await mockMetrics(page, "ready", hits);
  await page.goto(STREAMS);
  await expect(page.locator(".mx-strip .mx-big")).toBeVisible();
  const n = hits.length;
  await page.evaluate(() => {
    Object.defineProperty(document, "hidden", { configurable: true, get: () => true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.waitForTimeout(2500);
  expect(hits.length).toBe(n);
});
