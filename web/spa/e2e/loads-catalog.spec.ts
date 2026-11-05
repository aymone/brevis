import { expect, test } from "./support";

// The Loads and Catalog lists answer a few questions per row (which load or destination, where it goes, is it
// healthy), the whole row is one link, and on a phone each row is a card. Both answers are mocked: the demo stack
// has few loads and destinations.
const place = (kind: string, where: string, what: string) => ({ uri: where + what, kind, where, what, note: "" });
const load = (i: number, fixing: boolean) => ({
  workflow: "orders", step: `load_${i}`, href: `/ingest/loads/orders/load_${i}`, target: place("postgres", "postgres://warehouse/public.", `orders_${i}`), legacy: false,
  status: "on_time", lastLoaded: "2026-11-01T10:00:00Z", rows: 1200, schedule: { $: "cat", v: ["every hour"] },
  source: { kind: "http", driver: "", label: "HTTP API", labelMsg: null, where: "api.example.com" }, recent: [1, 2, 3, 2], fixing: fixing ? "run1" : null,
});
const loadsAnswer = {
  rows: [load(0, false), load(1, false), load(2, true)], total: 3, workflows: 1, summary: { on_time: 3 }, counts: { "": 3, on_time: 3 },
  sources: [{ key: "http", label: { k: "loads.source.http" } }], hasLegacy: false, filter: { status: "", source: "", q: "", legacy: false },
};
const dest = (i: number) => ({
  target: `postgres://warehouse/public.orders_${i}`, kind: "postgres", legacy: false, status: "on_time", where: "postgres://warehouse/public.", what: `orders_${i}`,
  autoTable: false, writer: "orders · load_orders", more: i === 1 ? 2 : 0, queued: false, lastAt: "2026-11-01T10:00:00Z", lastRows: 1200, recent: [1, 2, 3], streamHref: "",
});
const catalogAnswer = { now: "2026-11-02T10:00:00Z", kinds: ["postgres"], hasLegacy: false, total: 3, shown: 3, counts: { all: 3, on_time: 3 }, rows: [dest(0), dest(1), dest(2)] };

test.beforeEach(async ({ page }) => {
  await page.route("**/api/ingest/loads", (r) => r.fulfill({ json: loadsAnswer }));
  await page.route("**/api/catalog", (r) => r.fulfill({ json: catalogAnswer }));
});

const screens = [
  { name: "loads", path: "/ingest/loads", rows: "#load-rows tbody tr", link: "a.ldl-link", opens: /\/ingest\/loads\/orders\/load_0$/, third: 3 },
  { name: "catalog", path: "/data", rows: "table.ctl tbody tr", link: "a.ctl-link", opens: /\/data\/target\?u=postgres/, third: 2 },
];

for (const s of screens) {
  test.describe(s.name, () => {
    test.beforeEach(async ({ page }) => {
      await page.goto(s.path);
      await expect(page.locator(s.rows).first().locator(s.link)).toBeVisible();
    });

    test("Tab reaches the row once, Enter opens it, a click anywhere on the row opens it too", async ({ page }) => {
      const stops = await page.locator(s.rows).evaluateAll((rs) => rs.map((r) => r.querySelectorAll("a, button, input, select, [tabindex]").length));
      // A load being fixed also links to its run, so that row has a second stop.
      expect(stops.slice(0, 2)).toEqual([1, 1]);
      await page.locator(s.rows).first().locator(s.link).focus();
      await page.keyboard.press("Tab");
      await expect(page.locator(s.rows).nth(1).locator(s.link)).toBeFocused();
      await page.keyboard.press("Shift+Tab");
      await page.keyboard.press("Enter");
      await expect(page).toHaveURL(s.opens);
      await page.goBack();
      const cell = await page.locator(s.rows).first().locator("td").nth(s.third).boundingBox();
      await page.mouse.click(cell!.x + cell!.width / 2, cell!.y + cell!.height / 2);
      await expect(page).toHaveURL(s.opens);
    });

    test("on a phone each row is a card and nothing scrolls sideways", async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      const tr = page.locator(s.rows).first();
      await expect(tr).toHaveCSS("display", "grid");
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
      await tr.click({ position: { x: 20, y: 90 } });
      await expect(page).toHaveURL(s.opens);
    });
  });
}

test("a load's row has one destination line with its source under it, and keeps its facts", async ({ page }) => {
  await page.goto("/ingest/loads");
  const first = page.locator("#load-rows tbody tr").first();
  await expect(first.locator(".ldl-to")).toContainText("postgres://warehouse/public.orders_0");
  await expect(first.locator(".ldl-from")).toContainText("HTTP API");
  await expect(first.locator(".ldl-from")).toContainText("api.example.com");
  await expect(first.locator("td").nth(1).locator("div[title]")).toHaveAttribute("title", /HTTP API \(api\.example\.com\) → postgres:\/\/warehouse\/public\.orders_0/);
  await expect(first.locator(".ldl-rows span")).toHaveAttribute("title", "1200 rows");
  await expect(page.locator("#load-rows tbody tr").nth(2).locator("a.ldl-fixing")).toHaveAttribute("href", "/runs/run1");
  // Every row is as tall as the others.
  expect(await page.locator("#load-rows tbody tr").evaluateAll((rs) => new Set(rs.map((r) => Math.round(r.getBoundingClientRect().height))).size)).toBe(1);
});

test("the catalog names the writer once, muted under the destination, and the notes sit under the table", async ({ page }) => {
  await page.goto("/data");
  const rows = page.locator("table.ctl tbody tr");
  await expect(rows.first().locator(".ctl-sub")).toContainText("orders · load_orders");
  await expect(rows.nth(1).locator(".ctl-sub")).toContainText("+2");
  await expect(rows.first().locator("td").nth(0).locator("img, svg").first()).toBeVisible();
  // The writer is no column of its own: five named columns and the chevron's.
  await expect(page.locator("table.ctl thead th")).toHaveCount(6);
  await expect(page.locator("table.ctl").locator("xpath=../../p")).toContainText("Writes made outside Brevis do not show.");
});
