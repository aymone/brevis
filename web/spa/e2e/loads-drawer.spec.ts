import type { Page } from "@playwright/test";
import { expect, loaded, test } from "./support";

// A load's address draws the loads list with the load in a drawer over it, as the old console did. The demo
// stack has few loads, so the API is answered here (only /api/ingest/loads*); everything else is the real one.
const place = (uri: string) => ({ uri, kind: "postgres", where: "postgres://warehouse/", what: uri.split("/").pop()!, note: "" });
const row = (workflow: string, step: string, uri: string, many = false) => ({
  workflow, step, href: `/ingest/loads/${workflow}/${step}${many ? `?target=${encodeURIComponent(uri)}` : ""}`, target: place(uri), legacy: false,
  status: "on_time", lastLoaded: "2026-11-01T10:00:00Z", rows: 1200, schedule: { $: "cat", v: ["every hour"] },
  source: { kind: "http", driver: "", label: "HTTP API", labelMsg: null, where: "api.example.com" }, recent: [1, 2, 3, 2], fixing: null,
});
const ROWS = [row("orders", "load_orders", "postgres://warehouse/public/orders"), row("users", "load_users", "postgres://warehouse/public/users"), row("fx", "load_fx", "postgres://warehouse/public/fx_a", true), row("fx", "load_fx", "postgres://warehouse/public/fx_b", true)];
const detail = (uri: string) => ({
  status: "on_time", reason: null, source: { kind: "http", driver: "", label: "HTTP API", labelMsg: null, where: "https://api.example.com/x" }, target: place(uri),
  hasSchedule: true, when: { $: "cat", v: ["every hour"] }, cron: "0 * * * *", timezone: "UTC", manual: false, legacy: false, lastLoaded: "2026-11-01T10:00:00Z", lastRows: 1200, read: null,
  readNothing: false, fixing: null, bars: [{ runId: "r1", rows: 5, title: "" }], missed: 0, loads: [{ loadedAt: "2026-11-01T10:00:00Z", rows: 5, bytes: 10, runId: "r1aaaaaaaa" }], loadsCount: { k: "loads.drawer.recent.count", n: 1 },
  described: false, config: null, changes: [], readBy: [], query: "", run: "",
});

async function mock(page: Page, seen: string[] = []) {
  await page.route("**/api/ingest/loads**", async (r) => {
    const u = new URL(r.request().url());
    seen.push(u.pathname + u.search);
    const parts = u.pathname.replace("/api/ingest/loads", "").split("/").filter(Boolean);
    if (parts.length === 0)
      return r.fulfill({ json: { rows: ROWS, total: 4, workflows: 3, summary: { on_time: 4 }, counts: { "": 4, on_time: 4 }, sources: [{ key: "http", label: { k: "loads.source.http" } }], hasLegacy: false, filter: { status: "", source: "", q: "", legacy: false } } });
    const [workflow, step] = parts.map(decodeURIComponent);
    const target = u.searchParams.get("target");
    if (workflow === "nope") return r.fulfill({ status: 404, json: { error: "not_found" } });
    if (step === "load_fx" && !target)
      return r.fulfill({ json: { workflow, step, choices: ["fx_a", "fx_b"].map((n) => ({ uri: `postgres://warehouse/public/${n}`, current: false })), choosing: true, detail: null } });
    const uri = target ?? ROWS.find((x) => x.step === step)!.target.uri;
    return r.fulfill({ json: { workflow, step, choices: step === "load_fx" ? ["fx_a", "fx_b"].map((n) => ({ uri: `postgres://warehouse/public/${n}`, current: `postgres://warehouse/public/${n}` === uri })) : [], choosing: false, detail: detail(uri) } });
  });
}
const drawer = (page: Page) => page.locator("dialog#load");

test("a deep link draws the list with the drawer open over it", async ({ page }) => {
  await mock(page);
  await page.goto("/ingest/loads/orders/load_orders");
  await expect(drawer(page)).toBeVisible();
  await expect(drawer(page).locator("#load-title")).toHaveText("load_orders");
  await expect(drawer(page).getByText("postgres://warehouse/public/orders")).toBeVisible();
  await expect(page.locator("#load-rows tbody tr")).toHaveCount(4);
  await expect(page).toHaveTitle(/Loads|loads/i);
});

test("Back closes a drawer a row opened, and the list keeps its filter", async ({ page }) => {
  const seen: string[] = [];
  await mock(page, seen);
  await page.goto("/ingest/loads?status=on_time");
  await loaded(page);
  seen.length = 0;
  await page.locator("#load-rows tbody tr").nth(1).click();
  await expect(drawer(page)).toBeVisible();
  await expect(page).toHaveURL(/\/ingest\/loads\/users\/load_users\?status=on_time$/);
  // One request for the detail, none for the list behind it.
  await expect(drawer(page).getByText("postgres://warehouse/public/users")).toBeVisible();
  expect(seen).toEqual(["/api/ingest/loads/users/load_users"]);
  await page.goBack();
  await expect(drawer(page)).toHaveCount(0);
  await expect(page).toHaveURL(/\/ingest\/loads\?status=on_time$/);
});

test("Escape closes the drawer and the focus goes back to the row", async ({ page }) => {
  await mock(page);
  await page.goto("/ingest/loads");
  await loaded(page);
  const link = page.locator("#load-rows a[data-load='users/load_users']");
  await link.focus();
  await page.keyboard.press("Enter");
  await expect(drawer(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(drawer(page)).toHaveCount(0);
  await expect(page).toHaveURL(/\/ingest\/loads$/);
  await expect(link).toBeFocused();
  // Opened by a click on the row, closed by the backdrop: the row's link still gets the focus.
  await page.locator("#load-rows tbody tr").nth(1).click();
  await expect(drawer(page)).toBeVisible();
  await page.mouse.click(20, 400);
  await expect(drawer(page)).toHaveCount(0);
  await expect(link).toBeFocused();
});

test("opening another load swaps the content of the same dialog", async ({ page }) => {
  await mock(page);
  await page.goto("/ingest/loads/orders/load_orders");
  await expect(drawer(page).getByText("postgres://warehouse/public/orders")).toBeVisible();
  await drawer(page).evaluate((d) => ((window as unknown as { __d: Element }).__d = d));
  // The page behind a modal cannot be pressed; the event is what a history jump or a shortcut would do.
  await page.locator("#load-rows tbody tr").nth(1).dispatchEvent("click");
  await expect(drawer(page).locator("#load-title")).toHaveText("load_users");
  await expect(drawer(page).getByText("postgres://warehouse/public/users")).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as { __d: Element }).__d === document.querySelector("dialog#load"))).toBe(true);
});

test("a step with several destinations asks which, inside the drawer", async ({ page }) => {
  await mock(page);
  await page.goto("/ingest/loads/fx/load_fx");
  const choices = drawer(page).locator("a[href*='target=']");
  await expect(choices).toHaveCount(2);
  await choices.nth(1).click();
  await expect(page).toHaveURL(/target=postgres/);
  await expect(drawer(page).getByText("postgres://warehouse/public/fx_b").first()).toBeVisible();
  await expect(drawer(page).locator("a[aria-current=true]")).toContainText("fx_b");
  await expect(drawer(page).locator("a[aria-current=true]")).toBeFocused();
});

test("a load that writes nothing is the list with the note", async ({ page }) => {
  await mock(page);
  await page.goto("/ingest/loads/nope/gone");
  await expect(page.getByText("nope · gone").first()).toBeVisible();
  await expect(drawer(page)).toHaveCount(0);
  await expect(page.locator("#load-rows tbody tr")).toHaveCount(4);
});

test("opening the drawer shifts nothing on the page behind it", async ({ page }) => {
  await mock(page);
  await page.addInitScript(() => {
    const w = window as unknown as { __cls: number };
    w.__cls = 0;
    new PerformanceObserver((l) => {
      for (const e of l.getEntries() as unknown as { hadRecentInput: boolean; value: number }[]) if (!e.hadRecentInput) w.__cls += e.value;
    }).observe({ type: "layout-shift", buffered: true });
  });
  await page.goto("/ingest/loads");
  await loaded(page);
  await page.waitForTimeout(300);
  const before = await page.evaluate(() => (window as unknown as { __cls: number }).__cls);
  const top = await page.locator("#load-rows").evaluate((e) => e.getBoundingClientRect().top);
  await page.locator("#load-rows tbody tr").first().click();
  await expect(drawer(page).getByText("postgres://warehouse/public/orders")).toBeVisible();
  await page.waitForTimeout(400);
  expect(await page.locator("#load-rows").evaluate((e) => e.getBoundingClientRect().top)).toBe(top);
  expect((await page.evaluate(() => (window as unknown as { __cls: number }).__cls)) - before).toBeLessThan(0.01);
});

test("Run now in the drawer starts the workflow with the JSON trigger and goes to the new run, with a toast", async ({ page }) => {
  await mock(page);
  // The detail says the workflow can run with one click (it has no parameters).
  await page.route("**/api/ingest/loads/orders/load_orders", (r) =>
    r.fulfill({ json: { workflow: "orders", step: "load_orders", choices: [], choosing: false, detail: { ...detail("postgres://warehouse/public/orders"), run: "now" } } }),
  );
  const posts: string[] = [];
  await page.route("**/api/workflows/orders/trigger", (r) => {
    posts.push(r.request().postData() ?? "");
    return r.fulfill({ json: { runId: "11111111-1111-1111-1111-111111111111", href: "/runs/11111111-1111-1111-1111-111111111111" } });
  });
  await page.goto("/ingest/loads/orders/load_orders");
  await drawer(page).getByRole("button", { name: "Run now", exact: true }).click();
  await expect(page).toHaveURL(/\/runs\/11111111-1111-1111-1111-111111111111$/);
  await expect(page.getByText("Run of orders created")).toBeVisible();
  expect(posts).toHaveLength(1);
  expect(JSON.parse(posts[0]!)).toEqual({ params: {} });
});
