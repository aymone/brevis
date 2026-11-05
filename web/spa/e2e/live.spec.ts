import { expect, loaded, quiet, test, trackApi, trigger } from "./support";

// The event stream replaced polling: an open screen asks the server nothing until the server
// says something changed, and then it asks once.

const IDLE_MS = 5_000;

test("an idle screen makes no REST requests", async ({ browser }) => {
  // The scratch audit watched 25 s per screen. This watches four screens at once for 5 s: it
  // catches a poll faster than that (the old 5 s and 10 s timers), not the 30 s fallback,
  // which only runs while the stream is down. Raise IDLE_MS to prove more.
  const ctx = await browser.newContext();
  // The Runs list follows the stream only when its Live switch is on (the old page's own key).
  await ctx.addInitScript(() => localStorage.setItem("brevis.runs.live", "on"));
  const first = await ctx.newPage();
  await quiet(first.request);
  const results: Record<string, string[]> = {};
  await Promise.all(
    ["/", "/runs", "/workflows", "/ingest/streams"].map(async (path) => {
      const page = await ctx.newPage();
      const api = trackApi(page);
      await page.goto(path);
      await loaded(page);
      await page.waitForTimeout(1000);
      api.take();
      await page.waitForTimeout(IDLE_MS);
      results[path] = api.take();
    }),
  );
  await ctx.close();
  expect(results).toEqual({ "/": [], "/runs": [], "/workflows": [], "/ingest/streams": [] });
});

test("a run started on the server refreshes the open Runs list once", async ({ page, seed }) => {
  await quiet(page.request);
  await page.addInitScript(() => localStorage.setItem("brevis.runs.live", "on"));
  await page.goto("/runs");
  await loaded(page);
  await expect(page.locator("main button[aria-pressed=true]")).toHaveCount(1); // Live is on
  await page.waitForTimeout(1000);
  // The rows only: with more than a page of runs the pager is a link to /runs/<id>?page=2 too.
  const rows = page.locator("#runs-list nav a[href*='/runs/']");
  const before = await rows.count();
  const api = trackApi(page);
  api.take();
  await trigger(page, seed.slug);
  await expect(rows).toHaveCount(Math.min(before + 1, 25));
  await page.waitForTimeout(1500);
  const reads = api.take().filter((r) => r.startsWith("/api/runs") && !/\/api\/runs\/[0-9a-f-]{36}/.test(r));
  // A queued run appearing and then starting are two changes the server may report; what must never
  // happen is a read per second.
  expect(reads.length, `reads of the list after one trigger: ${reads.join(", ")}`).toBeLessThanOrEqual(3);
  expect(reads.length).toBeGreaterThanOrEqual(1);
  await quiet(page.request);
});

test("the page opens one event stream and keeps it while moving between screens that share topics", async ({ page }) => {
  const streams: string[] = [];
  page.on("request", (r) => /\/api\/events/.test(r.url()) && streams.push(r.url()));
  await page.goto("/workflows");
  await loaded(page);
  for (const to of ["/data", "/ingest/loads", "/workflows"]) {
    await page.locator(`nav a[href="${to}"], nav a[href="${to.replace(/\/$/, "")}"]`).first().click();
    await loaded(page);
  }
  await page.waitForTimeout(800);
  // The shared topics are always on the stream, so these screens never reopen it (connection.ts SHARED).
  // A run's page adds a named topic (run:<id>), and the Overview and Streams screens add metrics:streams while
  // they are open: both reopen it, by design, and are not walked here.
  expect(streams).toHaveLength(1);
});
