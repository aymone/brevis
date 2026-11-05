import { expect, loaded, test } from "./support";

test("workflows: the search narrows the list and says so when nothing matches", async ({ page, seed }) => {
  await page.goto("/workflows");
  await loaded(page);
  const rows = page.locator("main tbody tr");
  const all = await rows.count();
  await page.getByRole("searchbox").fill(seed.slug);
  await expect(page).toHaveURL(new RegExp(`q=${seed.slug}`));
  await expect(rows.first()).toContainText(seed.slug);
  expect(await rows.count()).toBeLessThanOrEqual(all);
  await page.getByRole("searchbox").fill("zzz-nothing");
  await expect(rows).toHaveCount(0);
  await expect(page.locator("main")).toContainText(/no workflow|nothing|found|none/i);
});

test("runs: '/' focuses the search, and opening a run shows it beside the list", async ({ page }) => {
  await page.goto("/runs");
  await loaded(page);
  await page.locator("body").click({ position: { x: 700, y: 700 } });
  await page.keyboard.press("/");
  await expect(page.locator("input:focus")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await page.locator("#runs-list a[href*='/runs/']").first().click();
  await expect(page).toHaveURL(/\/runs\/[0-9a-f-]{36}/);
  await expect(page.locator("#run-pane")).not.toBeEmpty();
});

test("the command palette opens with Ctrl+K, finds a workflow by name and goes there", async ({ page, seed }) => {
  await page.goto("/");
  await loaded(page);
  await page.keyboard.press("Control+k");
  const dialog = page.locator("dialog[open], [role=dialog]").first();
  await expect(dialog).toBeVisible();
  await page.keyboard.type(seed.slug);
  await expect(dialog).toContainText(seed.slug);
  await page.keyboard.press("Enter");
  await expect(new URL(page.url()).pathname).not.toBe("/");
});

test("workflows: pausing and resuming a schedule toggles the switch (and leaves it as it was)", async ({ page }) => {
  await page.goto("/workflows");
  await loaded(page);
  const sw = page.locator('main [role="switch"]').first();
  test.skip((await sw.count()) === 0, "precondition absent: no scheduled workflow, so no switch");
  const was = await sw.getAttribute("aria-checked");
  try {
    await sw.click();
    await expect(sw).not.toHaveAttribute("aria-checked", was!);
  } finally {
    if ((await sw.getAttribute("aria-checked")) !== was) await sw.click();
  }
  await expect(sw).toHaveAttribute("aria-checked", was!);
});

test("the other list screens write their search to the address (where there is something to search)", async ({ page }) => {
  for (const path of ["/data", "/ingest/streams", "/data/endpoints", "/ingest/loads"]) {
    await page.goto(path);
    await loaded(page);
    const box = page.locator('main input:not([type="hidden"]):not([type="checkbox"])').first();
    if ((await box.count()) === 0) continue; // nothing published there (no streams, no endpoints): no search to test
    await box.fill("x");
    await expect(page, `${path}: typing updates the url`).toHaveURL(/[?&](q|search)=x/);
  }
});

test("the run-now drawer validates a parameter before posting", async ({ page, seed }) => {
  test.skip(!seed.withParams, "precondition absent: no workflow with parameters (publish examples/list-params.yaml)");
  await page.goto(`/workflows/${seed.withParams}`);
  await loaded(page);
  await page.getByRole("button", { name: /run/i }).first().click();
  const dialog = page.locator("dialog[open]");
  await expect(dialog).toBeVisible();
  // `days` is list|integer: one bad item is refused naming the field, and nothing is sent.
  const posts: string[] = [];
  page.on("request", (r) => r.method() === "POST" && posts.push(r.url()));
  const days = dialog.locator('[data-param="days"] input').first();
  await days.fill("1,x,30");
  await days.press("Enter");
  await expect(dialog.locator("[data-param-error]").first()).toBeVisible();
  expect(posts.filter((u) => u.endsWith("/trigger"))).toEqual([]);
});

// The server answers a refused slot as a code and data; the drawer says it in the reader's language
// (internal/api/trigger_api.go, web/spa/src/api/trigger.ts). 04:30 is not a fire of e2e_nightly's "0 4 * * *".
for (const [lang, sentence] of [["en", /is not a slot of "0 4 \* \* \*"/], ["pt-BR", /não é um slot de "0 4 \* \* \*"/]] as const) {
  test(`the run-now drawer says a refused slot in ${lang}`, async ({ browser, baseURL, seed }) => {
    void seed;
    const ctx = await browser.newContext();
    await ctx.addCookies([{ name: "brevis_lang", value: lang, url: baseURL ?? "http://localhost:8080" }]);
    const page = await ctx.newPage();
    await page.goto("/workflows/e2e_nightly");
    await loaded(page);
    await page.getByRole("button", { name: /run|executar/i }).first().click();
    const dialog = page.locator("dialog[open]");
    await expect(dialog).toBeVisible();
    await dialog.locator('input[name="when"][value="slot"]').check({ force: true });
    await dialog.getByRole("combobox").selectOption("other");
    await dialog.locator('input[type="datetime-local"]').fill("2024-01-02T04:30");
    await dialog.locator('button[type="submit"]').click();
    const err = dialog.locator("[data-slot-error]");
    await expect(err).toBeVisible();
    await expect(err).toHaveText(sentence);
    await expect(err).not.toContainText("slot_not_on_schedule");
    await ctx.close();
  });
}
