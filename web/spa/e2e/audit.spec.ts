import { expect, routes, loaded, test } from "./support";

// Every screen, at every size, in both themes: the checks that were done by eye and kept
// coming back (a console error, a 4xx, a sideways scrollbar, an untranslated [key], "NaN").
// The matrix is a diagonal, not the full product: each size and each theme is covered, and
// the suite stays inside its time budget.
const matrix = [
  { w: 1440, h: 900, theme: "dark" },
  { w: 1024, h: 768, theme: "light" },
  { w: 768, h: 1024, theme: "dark" },
  { w: 390, h: 844, theme: "light" },
] as const;

for (const m of matrix) {
  test(`every screen is clean at ${m.w}x${m.h}, ${m.theme}`, async ({ browser, seed }) => {
    const ctx = await browser.newContext({ viewport: { width: m.w, height: m.h }, colorScheme: m.theme });
    const page = await ctx.newPage();
    const errors: string[] = [];
    const failed: string[] = [];
    page.on("pageerror", (e) => errors.push(String(e).slice(0, 160)));
    page.on("console", (c) => c.type() === "error" && errors.push(`console: ${c.text().slice(0, 160)}`));
    // The SQL tab answers 4xx on purpose when no SQL service is configured.
    page.on("response", (r) => r.status() >= 400 && !/\/api\/sql\//.test(r.url()) && failed.push(`${r.status()} ${new URL(r.url()).pathname}`));

    const problems: string[] = [];
    for (const route of routes(seed)) {
      errors.length = failed.length = 0;
      await page.goto(route);
      await loaded(page).catch((e: Error) => problems.push(`${route}: not loaded (${e.message.split("\n")[0]})`));
      const res = await page.evaluate(async () => {
        await document.fonts.ready;
        const text = document.body.innerText;
        return {
          over: document.documentElement.scrollWidth - document.documentElement.clientWidth,
          keys: text.match(/\[[a-z]+(\.[a-z_0-9]+)+\]/g)?.slice(0, 3) ?? [],
          junk: text.match(/\bNaN\b|\bundefined\b|\[object |Invalid Date/g)?.slice(0, 3) ?? [],
          font: document.fonts.check('14px "IBM Plex Sans"'),
          empty: (document.querySelector("main")?.innerText ?? "").trim().length < 20,
        };
      });
      if (res.over > 0) problems.push(`${route}: sideways overflow ${res.over}px`);
      if (res.keys.length) problems.push(`${route}: untranslated ${res.keys}`);
      if (res.junk.length) problems.push(`${route}: junk text ${res.junk}`);
      if (!res.font) problems.push(`${route}: IBM Plex Sans did not load`);
      if (res.empty) problems.push(`${route}: <main> is empty`);
      if (errors.length) problems.push(`${route}: errors ${errors.slice(0, 2).join(" | ")}`);
      if (failed.length) problems.push(`${route}: failed requests ${[...new Set(failed)].join(", ")}`);
    }
    await ctx.close();
    expect(problems).toEqual([]);
  });
}

test("the SQL screen renders (or says there is no SQL service)", async ({ page, seed }) => {
  test.skip(!seed.sql, "precondition absent: no SQL service configured (BREVIS_SQL_SERVE_URL)");
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.goto("/sql");
  await expect(page.locator("main h1").first()).toBeVisible();
  expect(errors).toEqual([]);
});

test("an unknown address says not found instead of a blank page", async ({ page }) => {
  await page.goto("/nope");
  await expect(page.getByText("Not found")).toBeVisible();
});
