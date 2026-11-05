import { expect, loaded, routes, test } from "./support";

test("the page title sits at the same place on every screen", async ({ browser, seed }) => {
  for (const [w, h] of [[1440, 900], [390, 844]] as const) {
    const ctx = await browser.newContext({ viewport: { width: w, height: h } });
    const page = await ctx.newPage();
    const at: Record<string, string> = {};
    // The run page and the workflow page have their own heads (a data title with a status); the
    // list and catalog screens share the PageFrame, and a title that moves between them is a bug.
    for (const route of routes(seed).filter((r) => !/\/runs\/|\/workflows\/[^/]+$|\/runs$/.test(r))) {
      await page.goto(route);
      await loaded(page);
      at[route] = await page.evaluate(() => {
        const b = document.querySelector("main h1")!.getBoundingClientRect();
        return `x${Math.round(b.left)} y${Math.round(b.top)}`;
      });
    }
    await ctx.close();
    const xs = new Set(Object.values(at).map((v) => v.split(" ")[0]));
    const ys = Object.values(at).map((v) => Number(v.split(" y")[1]));
    expect(xs.size, `${w}px: ${JSON.stringify(at)}`).toBe(1);
    // Desktop: exact. On a phone the Workflows head sits 4 px higher than the rest (known; its actions wrap
    // differently), so that is the tolerance, not a licence for more.
    expect(Math.max(...ys) - Math.min(...ys), `${w}px: ${JSON.stringify(at)}`).toBeLessThanOrEqual(w < 600 ? 4 : 0);
  }
});

for (const [w, h] of [[1440, 900], [1024, 768], [390, 844]] as const) {
  test(`the runs list scrolls inside its own box, not the page (${w}x${h})`, async ({ browser }) => {
    const ctx = await browser.newContext({ viewport: { width: w, height: h } });
    const page = await ctx.newPage();
    await page.goto("/runs");
    await loaded(page);
    const m = await page.evaluate(() => {
      const l = document.querySelector("#runs-list")!;
      return { doc: document.documentElement.scrollHeight, vp: innerHeight, over: document.documentElement.scrollWidth - document.documentElement.clientWidth, list: l.scrollHeight > l.clientHeight };
    });
    await ctx.close();
    expect(m.over).toBeLessThanOrEqual(0);
    // On a desktop the list is the scroller; on a phone the page may grow with it.
    if (w >= 1024 && m.list) expect(m.doc).toBeLessThanOrEqual(m.vp + 1);
  });
}

// The drawers are addressable: a link with a hash opens one, and Escape closes it.
for (const [url, label] of [["/workflows#how-to-create", "how to create"], ["/ingest/streams#novo", "new stream"], ["/data/endpoints#publicar", "publish endpoints"]] as const) {
  test(`drawer opens from its address and Escape closes it: ${label}`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(String(e)));
    await page.goto(url);
    await expect(page.locator("dialog[open]")).toHaveCount(1);
    await page.keyboard.press("Escape");
    await expect(page.locator("dialog[open]")).toHaveCount(0);
    expect(errors).toEqual([]);
  });
}
