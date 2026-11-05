import { expect, loaded, test } from "./support";

// A search with no result once threw React error #300 (a hook after an early return) and
// blanked the whole Workflows page. The unit test pins the component; this one pins the
// real screens, because the same shape can hide in any list.
const screens = ["/workflows", "/runs", "/data", "/ingest/streams"];

for (const path of screens) {
  test(`${path}: an empty search keeps the page, and clearing it brings the list back`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(String(e).slice(0, 160)));
    await page.goto(path);
    await loaded(page);
    const box = page.locator("main input[type=search], main input[placeholder*='earch' i], main input[placeholder*='workflow' i]").first();
    // The streams screen draws its search only once there is a stream to search.
    test.skip((await box.count()) === 0, `precondition absent: ${path} has nothing to search`);
    await box.fill("zzz-nothing-matches");
    await expect(page).toHaveURL(/[?&](q|workflow|search)=zzz/);
    await expect(page.locator("main h1").first()).toBeVisible();
    await expect(box).toBeVisible();
    await box.fill("");
    await expect(page).not.toHaveURL(/zzz/);
    await loaded(page);
    expect(errors).toEqual([]);
  });
}
