import { expect, loaded, mockStreamsList, test } from "./support";

// A link written before the drawers' ids were English still opens the drawer: bookmarks and links people shared name
// "#publicar" (the endpoints' publish drawer) and "#novo" (the streams' new-stream drawer). See `drawerOf` in ui/drawer.tsx.
test("an old #novo link opens the new-stream drawer", async ({ page }) => {
  await mockStreamsList(page);
  await page.goto("/ingest/streams#novo");
  await loaded(page);
  await expect(page.locator("dialog[open]")).toBeVisible();
  await expect(page.locator("dialog[open]")).toContainText("gateway");
});

test("an old #publicar link opens the endpoints' publish drawer", async ({ page }) => {
  await page.goto("/data/endpoints#publicar");
  await expect(page.locator("main h1").first()).toBeVisible();
  await expect(page.locator("dialog[open]")).toBeVisible();
  await expect(page.locator("dialog[open]")).toContainText("brevis endpoints publish");
});

test("the current ids still work", async ({ page }) => {
  await page.goto("/data/endpoints#publish");
  await expect(page.locator("dialog[open]")).toBeVisible();
});
