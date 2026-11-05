import { expect, test } from "./support";

// THE SERVER SENDS WHICH SENTENCE AND THE DATA; THE APP SAYS IT. The run's phases, why and alerts, and the
// overview's event log, from mocked JSON in the new shape (the demo stack has no failing run to read).
const at = "2026-10-15T04:00:00Z";
const run = {
  id: "11111111-1111-1111-1111-111111111111", workflow: "daily", logos: [], status: "failed", attempt: 0, inFlight: false, terminal: false,
  canceling: false, trigger: "schedule", slot: at, createdAt: at, startedAt: "2026-10-15T04:00:03Z", finishedAt: "2026-10-15T04:01:33Z",
  durationMs: 90_000, rerun: "", err: "boom",
  phases: [
    { key: "scheduled", label: { k: "run.phase.scheduled" }, detail: { kind: "slot", at }, state: "done" },
    { key: "started", label: { k: "run.phase.started" }, detail: { kind: "late", ms: 3000 }, state: "done" },
    { key: "running", label: { k: "run.phase.running" }, detail: { kind: "ran", ms: 90_000 }, state: "done" },
    { key: "finished", label: { k: "run.phase.finished" }, detail: { kind: "ended", state: "failed", at: "2026-10-15T04:01:33Z" }, state: "failed" },
  ],
  why: { title: { k: "run.why.step_exit", a: { step: "load", code: 3 } }, streak: 1, excerpt: "boom", hasLine: false },
  wait: null, step: null,
  auto: [
    { key: "adjusted_at", note: { k: "auto.adjusted_at" }, value: "", at, delaySeconds: null },
    { key: "delay", note: { k: "auto.delay" }, value: "", at: null, delaySeconds: 0 },
  ],
  params: [], steps: [],
  alerts: [{ tone: "failed", wording: { k: "run.alert.undelivered", a: { subject: { k: "run.alert.step", a: { step: "load" } } } }, channel: "slack", attempts: 2, err: "", createdAt: at, deliveredAt: null }],
  context: [{ step: "load", json: "", bytes: 0, note: { k: "run.context.failed" } }],
  log: { lines: 0, causeN: 0, blocks: [] },
};

test("a run's phases, why and alerts are said by the app from codes and data", async ({ page }) => {
  await page.route(`**/api/runs/${run.id}`, (r) => r.fulfill({ json: run }));
  await page.goto(`/runs/${run.id}`);
  const strip = page.getByRole("list", { name: "Timeline" });
  await expect(strip).toContainText("Scheduled");
  await expect(strip).toContainText("slot 2026-10-15 04:00:00 UTC");
  await expect(strip).toContainText("+3.0s");
  await expect(strip).toContainText("1m 30s");
  await expect(strip).toContainText("failed · 2026-10-15 04:01:33 UTC");
  await expect(page.getByRole("heading", { name: "Step load failed (exit 3)" })).toBeVisible();
  await page.goto(`/runs/${run.id}#params`);
  await expect(page.getByText("on time")).toBeVisible();
});

test("the event log puts the subject where the language wants it and names the source", async ({ page }) => {
  await page.route(/\/api\/overview\?/, async (r) => {
    const res = await r.fetch();
    const body = await res.json();
    body.events = [{ at, source: "stream", what: { k: "event.stream.published" }, subject: "gw/v1/x", actor: "", href: "", state: "" }];
    await r.fulfill({ json: body });
  });
  await page.goto("/#events");
  const row = page.locator("[data-event=stream]");
  await expect(row).toContainText("Stream gw/v1/x published");
  await expect(row).toContainText("Real time");
});
