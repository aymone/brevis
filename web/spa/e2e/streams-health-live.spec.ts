import { expect, test } from "./support";
import type { Page } from "@playwright/test";

// A stream's health from the gateway's measured series, in every state, on the list and the page, and followed
// live: the console emits `metrics:streams` once per scrape and both screens refetch their own query, once, with
// no skeleton and no shift. Mocked: the demo stack has no traffic source. A fake EventSource stands in for the
// stream so the spec decides when "a scrape finished".

type Health = Record<string, unknown> & { state: string; reason: string };
const place = (uri: string) => ({ uri, kind: "postgres", kindTitle: "", where: uri.slice(0, 20), what: uri.slice(20), note: "" });
const row = (i: number, health: Health) => ({
  gateway: "events_gateway", path: `/v1/stream_${i}`, href: `/ingest/streams/events_gateway/v1/stream_${i}`, note: "A stream.",
  sink: place(`postgres://warehouse/sales.events_${i}`), batch: "1 second or 500", batchEvery: "1s", batchRecords: 500, deadLetter: null, health,
});
const counts = (hs: Health[]) => hs.reduce<Record<string, number>>((a, h) => ({ ...a, [h.state]: (a[h.state] ?? 0) + 1 }), {});
const list = (hs: Health[]) => ({
  rows: hs.map((h, i) => row(i, h)), total: hs.length, gateways: 1, withDeadLetter: 0, healthCounts: counts(hs), kinds: [], filter: { kind: "", q: "" },
});
const detail = (health: Health) => ({
  gateway: "events_gateway", path: "/v1/stream_0", health, sink: place("postgres://warehouse/sales.events_0"), deadLetter: null,
  publishedAt: "2026-11-01T10:00:00Z", format: "json", lede: "Takes events.", summary: "Events land.", flow: null, tables: [],
  catalogHref: "", query: "", readBy: [], hasConfig: false, cards: [], yaml: "", changes: [], deadLetterPanel: null, buriedMetric: "",
  testEvent: { format: "json", body: "{}", curlHead: "", lands: "", table: "", allow: [], dataset: "" },
});
const win = (o: Record<string, number> = {}) => ({ received: 0, rejected: 0, dropped: 0, deadLetter: 0, delivered: 0, retried: 0, buried: 0, saturated: 0, ...o });
// A FIXED CLOCK, at the hour the time is widest ("11:18 AM": four digits): "since 9:07 PM" is a character
// shorter and wraps one line less, so the spec passed or failed with the hour it ran at.
const NOW = new Date("2026-11-04T11:30:00Z");
const SINCE = new Date(NOW.getTime() - 12 * 60_000).toISOString();
test.use({ timezoneId: "UTC", locale: "en-US" });
test.beforeEach(({ page }) => page.clock.setFixedTime(NOW));

const H = {
  healthy: { state: "healthy", reason: "delivering", windowMinutes: 5, window: win({ received: 900, delivered: 5 }) },
  quiet: { state: "quiet", reason: "no_traffic", windowMinutes: 5, since: SINCE },
  degraded: { state: "degraded", reason: "rejected_some", windowMinutes: 5, rejectedRatio: 0.37, window: win({ received: 63, rejected: 37 }) },
  failing: { state: "failing", reason: "delivery_failing", windowMinutes: 5, failedBatchRatio: 1, since: SINCE, window: win({ buried: 3 }) },
  unknown: { state: "unknown", reason: "source_unreachable" },
} satisfies Record<string, Health>;

// What the page shows for each of the state's reasons: the sentence says the number that justifies it.
const SENTENCES: [Health, RegExp][] = [
  [H.healthy, /none was refused, retried or buried in the last 5 minutes/],
  [H.quiet, /Nothing received in the last 5 minutes\. The last events came in at/],
  [{ state: "quiet", reason: "no_traffic_seen", observedMinutes: 42 }, /No events seen in the last 42 minutes/],
  [H.degraded, /37% of the last 5 minutes' events were rejected/],
  [{ state: "degraded", reason: "dead_lettered", window: win({ deadLetter: 1 }) }, /1 event went to the dead letter in the last 5 minutes/],
  [{ state: "degraded", reason: "dead_lettered", window: win({ buried: 2 }) }, /2 batches could not be delivered and were buried/],
  [{ state: "degraded", reason: "saturated", window: win({ saturated: 4 }) }, /refused 4 requests/],
  [{ state: "degraded", reason: "retrying", retryRatio: 0.4 }, /40% of the last 5 minutes' delivery attempts had to be retried/],
  [{ state: "degraded", reason: "slow_delivery", latencyP95Sec: 41.2 }, /took up to 41 seconds \(p95\)/],
  [{ state: "degraded", reason: "replicas_partial", replicas: 3, reachable: 2 }, /Only 2 of 3 gateway replicas answer/],
  [{ state: "failing", reason: "rejected_most", rejectedRatio: 0.8 }, /Most events are being rejected: 80%/],
  [{ state: "failing", reason: "dead_letter_most", deadLetterRatio: 0.9 }, /going to the dead letter: 90%/],
  [H.failing, /100% of the last 5 minutes' batches were buried/],
  [{ state: "failing", reason: "saturating" }, /buffer has been full for most of the last 5 minutes/],
  [{ state: "unknown", reason: "source_not_configured" }, /no metrics address for this stream's gateway/],
  [H.unknown, /have not answered for the last few scrapes/],
  [{ state: "unknown", reason: "waiting" }, /second scrape/],
  [{ state: "unknown", reason: "not_in_source" }, /does not report this stream/],
  [{ state: "unknown", reason: "not_reported" }, /has no name in its published configuration/],
];

const SHAPE = { healthy: "st-dot", quiet: "st-ring", degraded: "st-triangle", failing: "st-diamond", unknown: "st-ring-dashed" };
const WORD = { healthy: "healthy", quiet: "quiet", degraded: "degraded", failing: "failing", unknown: "no data" };

async function fakeStream(page: Page) {
  await page.addInitScript(() => {
    const all: any[] = [];
    (window as any).EventSource = class {
      static CLOSED = 2;
      readyState = 0;
      l: Record<string, ((e: unknown) => void)[]> = {};
      onerror: unknown = null;
      constructor(public url: string) {
        all.push(this);
        setTimeout(() => {
          this.readyState = 1;
          this.l.ready?.forEach((f) => f({}));
        }, 10);
      }
      addEventListener(t: string, f: (e: unknown) => void) {
        (this.l[t] ??= []).push(f);
      }
      close() {
        this.readyState = 2;
      }
    };
    (window as any).__emit = (topic: string) => all.forEach((s) => s.l.change?.forEach((f: any) => f({ data: JSON.stringify({ topic }) })));
    // A skeleton or a busy list that appears AFTER the first answer is a flicker; layout shifts add up.
    (window as any).__flicker = 0;
    (window as any).__cls = 0;
    (window as any).__who = [];
    new PerformanceObserver((l) => {
      for (const e of l.getEntries() as any[]) {
        if (e.hadRecentInput) continue;
        (window as any).__cls += e.value;
        // What moved, so a failure names the element and not just a number.
        (window as any).__who.push(`${e.value.toFixed(3)} ${e.sources.map((s: any) => s.node?.className || s.node?.nodeName).join(" | ")}`.slice(0, 160));
      }
    }).observe({ type: "layout-shift", buffered: true });
    addEventListener("DOMContentLoaded", () => {
      new MutationObserver(() => {
        if ((window as any).__armed && document.querySelector(".sk-ghost, [aria-busy=true]")) (window as any).__flicker++;
      }).observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["class", "aria-busy"] });
    });
  });
}
const emit = (page: Page, topic = "metrics:streams") => page.evaluate((t) => (window as any).__emit(t), topic);
const arm = (page: Page) => page.evaluate(() => ((window as any).__armed = true, (window as any).__cls = 0, (window as any).__who = []));

test("the list says each state in its own shape and word, and counts them in the summary", async ({ page }) => {
  const hs = [H.healthy, H.unknown, H.failing, H.quiet, H.degraded];
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: list(hs) }));
  await page.goto("/ingest/streams");
  const rows = page.locator("#stream-rows tbody tr");
  await expect(rows.first().locator("a.stl-link")).toBeVisible();
  // Failing first, then degraded, then the rest in the order they came.
  const order = ["failing", "degraded", "healthy", "unknown", "quiet"] as const;
  for (const [i, state] of order.entries()) {
    const mark = rows.nth(i).locator(".stl-state .st");
    await expect(mark).toHaveText(WORD[state]);
    await expect(mark).toHaveClass(new RegExp(SHAPE[state]));
  }
  await expect(rows.nth(0).locator(".stl-state").locator("xpath=.//*[@title]").first()).toHaveAttribute("title", /100% of the last 5 minutes' batches were buried/);
  const sum = page.locator(".stl-sum");
  await expect(sum).toContainText("1 failing");
  await expect(sum).toContainText("1 degraded");
  await expect(sum).toContainText("1 healthy");
  await expect(sum).toContainText("1 quiet");
  await expect(sum).toContainText("1 no data");
  expect(await sum.innerText()).toMatch(/failing[\s\S]*degraded[\s\S]*healthy[\s\S]*quiet[\s\S]*no data/);
});

for (const [i, [h, sentence]] of SENTENCES.entries()) {
  test(`the stream page says ${h.state} and why (${h.reason}, ${i})`, async ({ page }) => {
    await page.route("**/api/ingest/streams/**", (r) => r.fulfill({ json: detail(h) }));
    await page.goto("/ingest/streams/events_gateway/v1/stream_0");
    await expect(page.getByTestId("health-reason")).toHaveText(sentence);
    const word = WORD[h.state as keyof typeof WORD];
    await expect(page.locator(".st", { hasText: word }).first()).toHaveClass(new RegExp(SHAPE[h.state as keyof typeof SHAPE]));
    if (h.since && h.state !== "quiet") await expect(page.getByTestId("health-reason")).toContainText("since");
  });
}

test("a scrape finishing changes the state on the list: one refetch of each query, no skeleton, no shift", async ({ page }) => {
  await fakeStream(page);
  let current = [H.healthy, H.healthy, H.quiet];
  const hits = { list: 0, metrics: 0 };
  await page.route("**/api/ingest/streams*", (r) => (hits.list++, r.fulfill({ json: list(current) })));
  await page.route("**/api/ingest/metrics/streams*", (r) => (hits.metrics++, r.fulfill({ json: { configured: false, since: SINCE, intervalSec: 5, window: { start: 0, minutes: 30 }, owners: [], total: { values: {}, covered: [], ratePerMin: {}, lastHour: {}, observedSec: 0, owner: "", name: "", present: false }, entities: [] } })));
  await page.goto("/ingest/streams");
  const marks = page.locator("#stream-rows tbody tr .stl-state .st");
  await expect(marks).toHaveCount(3);
  await expect(page.locator(".stl-sum")).toContainText("2 healthy");
  await page.waitForTimeout(600);
  await arm(page);
  const before = { ...hits };
  current = [H.healthy, H.failing, H.quiet];
  await emit(page);
  await expect(page.locator(".stl-sum")).toContainText("1 failing");
  // The failing stream moved to the top, and its row says why.
  await expect(marks.first()).toHaveText("failing");
  await page.waitForTimeout(500);
  expect(hits.list - before.list, "list refetches for one event").toBe(1);
  expect(hits.metrics - before.metrics, "metrics refetches for one event").toBe(1);
  expect(await page.evaluate(() => (window as any).__flicker), "a skeleton or busy list appeared").toBe(0);
  expect(await page.evaluate(() => (window as any).__cls), "layout shift").toBeLessThan(0.01);
  // And it recovers the same way.
  current = [H.healthy, H.healthy, H.quiet];
  await emit(page);
  await expect(page.locator(".stl-sum")).not.toContainText("failing");
});

test("a state change that moves no row shifts nothing: the words differ in width and the cells do not", async ({ page }) => {
  await fakeStream(page);
  let current = [H.healthy, H.quiet, H.unknown];
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: list(current) }));
  await page.route("**/api/ingest/metrics/streams*", (r) => r.fulfill({ json: { configured: false, since: SINCE, intervalSec: 5, window: { start: 0, minutes: 30 }, owners: [], total: { values: {}, covered: [], ratePerMin: {}, lastHour: {}, observedSec: 0, owner: "", name: "", present: false }, entities: [] } }));
  await page.goto("/ingest/streams");
  await expect(page.locator("#stream-rows tbody tr .stl-state .st")).toHaveCount(3);
  await page.waitForTimeout(600);
  await arm(page);
  for (const next of [[H.quiet, H.healthy, H.unknown], [H.unknown, H.quiet, H.healthy], [H.healthy, H.quiet, H.unknown]]) {
    current = next;
    await emit(page);
    await expect(page.locator("#stream-rows tbody tr .stl-state .st").first()).toHaveText(WORD[next[0]!.state as keyof typeof WORD]);
  }
  expect(await page.evaluate(() => (window as any).__cls), "layout shift").toBeLessThan(0.01);
});

test("rows keep their places while the pointer is over the table, and settle when it leaves", async ({ page }) => {
  await fakeStream(page);
  let current = [H.healthy, H.healthy, H.healthy];
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: list(current) }));
  await page.route("**/api/ingest/metrics/streams*", (r) => r.fulfill({ json: { configured: false, since: SINCE, intervalSec: 5, window: { start: 0, minutes: 30 }, owners: [], total: { values: {}, covered: [], ratePerMin: {}, lastHour: {}, observedSec: 0, owner: "", name: "", present: false }, entities: [] } }));
  await page.goto("/ingest/streams");
  const paths = () => page.locator("#stream-rows tbody tr .stl-path").allInnerTexts();
  await expect.poll(paths).toEqual(["/v1/stream_0", "/v1/stream_1", "/v1/stream_2"]);
  await page.locator("#stream-rows tbody tr").nth(0).hover();
  current = [H.healthy, H.healthy, H.failing];
  await emit(page);
  await expect(page.locator(".stl-sum")).toContainText("1 failing"); // the data arrived
  await expect(page.locator("#stream-rows tbody tr").nth(2).locator(".st")).toHaveText("failing");
  expect(await paths()).toEqual(["/v1/stream_0", "/v1/stream_1", "/v1/stream_2"]); // but nobody moved under the pointer
  await page.mouse.move(5, 5);
  await expect.poll(paths).toEqual(["/v1/stream_2", "/v1/stream_0", "/v1/stream_1"]);
});

for (const vp of [{ width: 1440, height: 900 }, { width: 390, height: 844 }]) {
  test.describe(`${vp.width}`, () => {
    test.use({ viewport: vp });
    test(`a scrape finishing changes the state on the stream page at ${vp.width}px: one refetch of each query, no skeleton, no shift`, async ({ page }) => {
      await fakeStream(page);
      let current: Health = H.healthy;
      let reads = 0;
      await page.route("**/api/ingest/streams/**", (r) => (reads++, r.fulfill({ json: detail(current) })));
      await page.route("**/api/ingest/metrics/streams/**", (r) => r.fulfill({ json: { configured: false, since: SINCE, intervalSec: 5, window: { start: 0, minutes: 60 }, owners: [], total: { values: {}, covered: [], ratePerMin: {}, lastHour: {}, observedSec: 0, owner: "", name: "", present: false }, entities: [{ values: {}, covered: [], ratePerMin: {}, lastHour: {}, observedSec: 0, owner: "", name: "", present: false, gateway: "events_gateway", path: "/v1/stream_0" }] } }));
      await page.goto("/ingest/streams/events_gateway/v1/stream_0");
      await expect(page.getByTestId("health-reason")).toHaveText(/none was refused/);
      await page.waitForTimeout(600);
      await arm(page);
      const before = reads;
      current = H.degraded;
      await emit(page);
      await expect(page.getByTestId("health-reason")).toHaveText(/37% of the last 5 minutes' events were rejected/);
      await page.waitForTimeout(500);
      expect(reads - before, "detail refetches for one event").toBe(1);
      expect(await page.evaluate(() => (window as any).__flicker), "a skeleton appeared").toBe(0);
      expect(await page.evaluate(() => (window as any).__cls), `layout shift: ${await page.evaluate(() => (window as any).__who.join("; "))}`).toBeLessThan(0.01);
      // A longer sentence with its suffixes (three lines on a phone) fits the room the paragraph kept.
      current = { ...H.failing, reason: "dead_letter_most", deadLetterRatio: 0.9, recovering: true };
      await emit(page);
      await expect(page.getByTestId("health-reason")).toContainText("since");
      await page.waitForTimeout(400);
      expect(await page.evaluate(() => (window as any).__flicker), "a skeleton appeared").toBe(0);
      expect(await page.evaluate(() => (window as any).__cls), "layout shift across three sentences").toBeLessThan(0.01);
    });
  });
}

test("no horizontal scroll at 390px with every state on the list and the page", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await ctx.newPage();
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: list([H.healthy, H.unknown, H.failing, H.quiet, H.degraded]) }));
  await page.route("**/api/ingest/streams/**", (r) => r.fulfill({ json: detail(H.failing) }));
  await page.goto("/ingest/streams");
  await expect(page.locator("#stream-rows tbody tr a.stl-link").first()).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
  await page.goto("/ingest/streams/events_gateway/v1/stream_0");
  await expect(page.getByTestId("health-reason")).toContainText("buried");
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
  await ctx.close();
});
