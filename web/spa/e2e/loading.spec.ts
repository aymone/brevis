import { expect, routes, test } from "./support";

// WHAT IS STATIC MUST NOT WAIT (boot.ts): a title, a column header or a button label is
// words from the catalog, and the catalog is in the cache before the first render. With the
// data API held for 2.5 s, the static parts must already be drawn, never a placeholder.
const KEEP = /\/api\/(session|config|messages|nav|palette|release|events)/;

test("static titles, headers and labels are drawn while the data is still loading", async ({ browser, seed }) => {
  const blank: string[] = [];
  await Promise.all(routes(seed).map(async (route) => {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await page.route("**/api/**", async (route) => {
    if (!KEEP.test(route.request().url())) await new Promise((r) => setTimeout(r, 2500));
    await route.continue().catch(() => {});
  });
    await page.goto(route, { waitUntil: "commit" });
    await expect(page.locator("nav").first()).toBeVisible();
    await page.waitForTimeout(500); // inside the 2.5 s the data is held
    const isRun = route.includes("/runs/");
    const probes = await page.evaluate((isRun) =>
      [...document.querySelectorAll("main h1, main h2, main th, main [role=tab], main label, main button")]
        .filter((el) => !el.textContent?.trim() && !el.getAttribute("aria-label") && !el.querySelector("svg,img"))
        // A header cell with a fixed narrow width (`w-8`) is a deliberate spacer (the icon column); a run's headings are its data.
        .filter((el) => !(el.tagName === "TH" && /\bw-\d+\b/.test(el.className)) && !(isRun && /^H[12]$/.test(el.tagName)))
        .map((el) => el.tagName.toLowerCase()),
      isRun);
    // The run page's own title is its data (the workflow's name); every other screen has a fixed one.
    const h1 = (await page.locator("main h1").first().textContent({ timeout: 1500 }).catch(() => ""))?.trim();
    if (probes.length) blank.push(`${route}: blank static elements ${probes.join(",")}`);
    if (!h1 && !route.includes("/runs/")) blank.push(`${route}: no title while loading`);
    await ctx.close();
  }));
  expect(blank).toEqual([]);
});
