import { expect, test } from "./support";

// The Streams list answers three questions per row (which stream, where it goes, is it healthy), the whole
// row is one link, and on a phone each row is a card. The answer is mocked: the demo stack has two streams.
const place = (kind: string, where: string, what: string) => ({ uri: where + what, kind, kindTitle: "", where, what, note: "" });
const row = (i: number, dead: boolean) => ({
  gateway: "events_gateway", path: `/v1/stream_${i}`, href: `/ingest/streams/${i}`, note: "Clicks go to a topic. And a second sentence.",
  sink: place("postgres", "postgres://warehouse/sales.", `events_${i}`), batch: "1 second or 500", batchEvery: "1s", batchRecords: 500,
  deadLetter: dead ? place("file", "file:///var/dead-letter/", String(i)) : null,
});
const answer = { rows: [row(0, true), row(1, false), row(2, true)], total: 3, gateways: 1, withDeadLetter: 2, kinds: [{ key: "postgres", label: "PostgreSQL" }], filter: { kind: "", q: "" } };

test.beforeEach(async ({ page }) => {
  await page.route("**/api/ingest/streams*", (r) => r.fulfill({ json: answer }));
  await page.goto("/ingest/streams");
  await expect(page.locator("#stream-rows tbody tr").first().locator("a.stl-link")).toBeVisible();
});

test("one tab stop per row; Enter opens the stream; a click anywhere on the row opens it too", async ({ page }) => {
  const stops = await page.locator("#stream-rows tbody tr").evaluateAll((rs) => rs.map((r) => r.querySelectorAll("a, button, input, select, [tabindex]").length));
  expect(stops).toEqual([1, 1, 1]);
  await page.locator("#stream-rows a.stl-link").first().focus();
  await page.keyboard.press("Tab");
  await expect(page.locator("#stream-rows tbody tr").nth(1).locator("a.stl-link")).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/ingest\/streams\/0$/);
  await page.goBack();
  const batch = await page.locator("#stream-rows tbody tr").nth(2).locator("td").nth(2).boundingBox();
  await page.mouse.click(batch!.x + batch!.width / 2, batch!.y + batch!.height / 2);
  await expect(page).toHaveURL(/\/ingest\/streams\/2$/);
});

test("the row keeps every fact: compact batch, destination, dead letter and the origin once", async ({ page }) => {
  const first = page.locator("#stream-rows tbody tr").first();
  await expect(first.locator(".stl-batch [aria-hidden]")).toHaveText("1s · 500");
  await expect(first.locator(".stl-batch .sr-only")).toHaveText("1 second or 500");
  await expect(first.locator(".stl-uri")).toContainText("postgres://warehouse/sales.events_0");
  await expect(first.locator(".stl-sub")).toHaveText("Clicks go to a topic.");
  await expect(first.locator(".stl-dl")).toHaveAttribute("title", /file:\/\/\/var\/dead-letter\/0/);
  await expect(page.locator("#stream-rows tbody tr").nth(1).locator(".stl-dl")).toHaveCount(0);
  // One gateway: named once in the summary, not on every row.
  await expect(page.locator(".stl-sum .stl-gw")).toHaveText("events_gateway");
  await expect(page.locator("#stream-rows .stl-gw")).toHaveCount(0);
  expect(await page.locator("#stream-rows tbody tr").evaluateAll((rs) => new Set(rs.map((r) => Math.round(r.getBoundingClientRect().height))).size)).toBe(1);
});

test("how streams work is open the first time and stays as the operator left it", async ({ page }) => {
  const how = page.locator("details.stl-how");
  await expect(how).toHaveJSProperty("open", true);
  await how.locator("summary").click();
  await expect(how).toHaveJSProperty("open", false);
  await expect.poll(() => page.evaluate(() => localStorage.getItem("brevis.seen.streams.how"))).toBe('{"open":false}');
  await page.reload();
  await expect(page.locator("details.stl-how")).toHaveJSProperty("open", false);
});

test("on a phone each stream is a card and nothing scrolls sideways", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const tr = page.locator("#stream-rows tbody tr").first();
  await expect(tr).toHaveCSS("display", "grid");
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  const path = await tr.locator(".stl-path").boundingBox();
  const dest = await tr.locator(".stl-kind").boundingBox();
  expect(dest!.y).toBeGreaterThan(path!.y);
  await tr.click({ position: { x: 20, y: 60 } });
  await expect(page).toHaveURL(/\/ingest\/streams\/0$/);
});
