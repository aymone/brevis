import { expect, test as base, type APIRequestContext, type Page } from "@playwright/test";

/**
 * What the suite needs on the server, found through the public API. The specs never
 * assume a particular demo: they take the first workflow that runs without parameters,
 * and trigger one run when the engine has none yet.
 */
export type Seed = { slug: string; runId: string; withParams: string | null; sql: boolean };

type WorkflowsList = { rows: { slug: string; hasParams: boolean; active: boolean }[] };

export async function seed(request: APIRequestContext): Promise<Seed | string> {
  const health = await request.get("/health").catch(() => null);
  if (!health?.ok()) return "no engine answers at BASE_URL (start it: see web/spa/README.md)";
  const wf = (await (await request.get("/api/workflows?perPage=100")).json()) as WorkflowsList;
  const plain = wf.rows.find((w) => !w.hasParams);
  if (!plain) return "no workflow without parameters is published (publish examples/hello.yaml)";
  const runs = async () => ((await (await request.get("/api/runs")).json()) as { runs?: { id: string }[] }).runs ?? [];
  if ((await runs()).length === 0) {
    const t = await request.post(`/api/workflows/${plain.slug}/trigger`, { data: { params: {} } });
    if (!t.ok()) return `triggering ${plain.slug} answered ${t.status()}`;
  }
  await expect.poll(async () => (await runs()).length, { message: "a run exists", timeout: 15_000 }).toBeGreaterThan(0);
  const cfg = (await (await request.get("/api/config")).json()) as { sql: boolean };
  return {
    slug: plain.slug,
    runId: (await runs())[0]!.id,
    // the drawer test types into the `days` list parameter that examples/list-params.yaml declares: any other
    // workflow with parameters (a demo someone published) would not have that field
    withParams: wf.rows.find((w) => w.hasParams && w.slug === "list_params")?.slug ?? null,
    sql: cfg.sql,
  };
}

/** `test` with `seed` available, and the whole file skipped (with the reason) when a precondition is absent. */
export const test = base.extend<{ seed: Seed; _gate: void }, { _seed: Seed | string }>({
  _seed: [async ({ playwright }, use) => {
    const ctx = await playwright.request.newContext({ baseURL: process.env.BASE_URL ?? "http://localhost:8080" });
    await use(await seed(ctx));
    await ctx.dispose();
  }, { scope: "worker" }],
  // Automatic, so a test that never asks for `seed` is skipped too, instead of failing on a dead URL.
  _gate: [async ({ _seed }, use, info) => {
    // CI sets E2E_REQUIRE: there, a missing precondition is the job's bug, and a skipped suite would be a green lie.
    if (typeof _seed === "string" && process.env.E2E_REQUIRE) throw new Error(`E2E_REQUIRE is set and a precondition is absent: ${_seed}`);
    if (typeof _seed === "string") info.skip(true, `precondition absent: ${_seed}`);
    await use();
  }, { auto: true }],
  seed: async ({ _seed }, use) => use(_seed as Seed),
});
export { expect };

export const routes = (s: Seed) => [
  "/", "/runs", `/runs/${s.runId}`, "/workflows", `/workflows/${s.slug}`, "/projects",
  "/data", "/data/endpoints", "/ingest/loads", "/ingest/streams",
];

/** A screen is loaded when its title is there and nothing in <main> is still a placeholder. */
export async function loaded(page: Page) {
  await expect(page.locator("main h1").first()).toBeVisible();
  await expect(page.locator("main .sk")).toHaveCount(0);
}

export const isMetricsRead = (path: string) => path.endsWith("/metrics") || path.startsWith("/api/ingest/metrics/");

/** The /api reads a page makes (the event stream is a connection, not a read). */
export function trackApi(page: Page) {
  let log: string[] = [];
  page.on("request", (r) => {
    const u = new URL(r.url());
    // The live volume reads (…/metrics) are a feed with a budget of their own, one per scrape: metrics.spec.ts
    // counts them. Here they would make "an idle screen asks nothing" depend on whether a source is configured.
    if (u.pathname.startsWith("/api/") && u.pathname !== "/api/events" && !isMetricsRead(u.pathname)) log.push(u.pathname + u.search);
  });
  return {
    take() {
      const out = log;
      log = [];
      return out;
    },
    get all() {
      return log;
    },
  };
}
export const duplicates = (reads: string[]) => {
  const n = new Map<string, number>();
  for (const r of reads) n.set(r, (n.get(r) ?? 0) + 1);
  return [...n].filter(([, c]) => c > 1).map(([k, c]) => `${k} x${c}`);
};

/** Starts a run through the public API, like `curl -X POST`. */
export async function trigger(page: Page, slug: string) {
  const r = await page.request.post(`/api/workflows/${slug}/trigger`, { data: { params: {} } });
  expect(r.ok(), `trigger ${slug}`).toBeTruthy();
}

/** Waits until no run is queued or running: a run in flight makes the stream say things, and the specs that count reads need silence. */
export async function quiet(request: APIRequestContext) {
  await expect
    .poll(async () => ((await (await request.get("/api/runs")).json()) as { counts: { going: number } }).counts.going, { timeout: 30_000, message: "runs in flight settle" })
    .toBe(0);
}

/** The Streams list with two published streams: the band and the "how streams work" box only exist when there is
 *  something listed, and a clean database (CI, a fresh checkout) has no published stream. */
export async function mockStreamsList(page: Page) {
  const place = (uri: string) => ({ uri, kind: "postgres", where: uri.slice(0, 20), what: uri.slice(20), note: "" });
  const row = (i: number) => ({
    gateway: "events_gateway", path: `/v1/${i ? "orders" : "clicks"}`, href: `/ingest/streams/events_gateway/v1/${i ? "orders" : "clicks"}`, note: "A stream.",
    sink: place(`postgres://warehouse/sales.events_${i}`), batch: { $: "cat", v: ["1 second or 500"] }, batchEvery: "1s", batchRecords: 500, deadLetter: null,
  });
  await page.route(/\/api\/ingest\/streams(\?.*)?$/, (r) =>
    r.fulfill({ json: { rows: [row(0), row(1)], total: 2, gateways: 1, withDeadLetter: 0, healthCounts: { unknown: 2 }, kinds: [], filter: { kind: "", q: "" } } }),
  );
}
