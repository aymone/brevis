import { expect, test } from "./support";

// The server holds a stream's configuration and no traffic, so its health is "unknown" and says why. The list
// and the page must say "no data" and the reason, never "continuous" (that is how a stream writes, not whether
// it works). Mocked: the demo stack has one reason only, and an older server has none.
const place = (uri: string) => ({ uri, kind: "postgres", where: uri.slice(0, 20), what: uri.slice(20), note: "" });
const row = (i: number, health?: { state: string; reason: string }) => ({
  gateway: "events_gateway", path: `/v1/stream_${i}`, href: `/ingest/streams/events_gateway/v1/stream_${i}`, note: "A stream.",
  sink: place(`postgres://warehouse/sales.events_${i}`), batch: { $: "cat", v: ["1 second or 500"] }, batchEvery: "1s", batchRecords: 500, deadLetter: null,
  ...(health ? { health } : {}),
});
const NOT_REPORTED = { state: "unknown", reason: "not_reported" };
const UNNAMED = { state: "unknown", reason: "destination_unnamed" };
const list = {
  rows: [row(0, NOT_REPORTED), row(1, UNNAMED), row(2)], total: 3, gateways: 1, withDeadLetter: 0,
  healthCounts: { unknown: 3 }, kinds: [], filter: { kind: "", q: "" },
};
const detail = (health: { state: string; reason: string }) => ({
  gateway: "events_gateway", path: "/v1/stream_0", health, sink: place("postgres://warehouse/sales.events_0"), deadLetter: null,
  publishedAt: "2026-11-01T10:00:00Z", format: "json", lede: { $: "cat", v: [{ k: "streams.page.lede", a: { dest: "PostgreSQL" } }] }, summary: { $: "cat", v: ["Events land."] }, flow: null, tables: [],
  catalogHref: "", query: "", readBy: [], hasConfig: false, cards: [], yaml: "", changes: [], deadLetterPanel: null, buriedMetric: "",
  testEvent: { format: "json", body: "{}", curlHead: "", lands: "", table: "", allow: [], dataset: "" },
});

test("the list says no data, never continuous, and counts it in the summary", async ({ page }) => {
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: list }));
  await page.goto("/ingest/streams");
  const rows = page.locator("#stream-rows tbody tr");
  await expect(rows.first().locator("a.stl-link")).toBeVisible();
  for (let i = 0; i < 3; i++) await expect(rows.nth(i).locator(".stl-state .st")).toHaveText("no data");
  await expect(rows.nth(0).locator(".stl-state .st")).toHaveClass(/st-ring-dashed/);
  await expect(rows.nth(1).locator(".stl-state").locator("xpath=.//*[@title]").first()).toHaveAttribute("title", /could not name where this stream lands/);
  await expect(page.locator(".stl-sum")).toContainText("3 no data");
  await expect(page.locator("#stream-rows")).not.toContainText("continuous");
  await expect(page.locator(".stl-sum")).not.toContainText("continuous");
});

for (const [reason, sentence] of [["not_reported", /has no name in its published configuration/], ["destination_unnamed", /could not name where this stream lands/]] as const) {
  test(`the stream page says no data and why (${reason})`, async ({ page }) => {
    await page.route("**/api/ingest/streams/**", (r) => r.fulfill({ json: detail({ state: "unknown", reason }) }));
    await page.goto("/ingest/streams/events_gateway/v1/stream_0");
    await expect(page.getByTestId("health-reason")).toHaveText(sentence);
    await expect(page.locator(".st", { hasText: "no data" })).toBeVisible();
    await expect(page.locator("main")).not.toContainText("continuous");
  });
}
