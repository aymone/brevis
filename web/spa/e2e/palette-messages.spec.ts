import { expect, loaded, test } from "./support";

// The server has no language: /api/palette sends a name (a string) or a sentence ({k, a}), and the palette says
// the sentence in the reader's language and searches what it says. The index is answered here; the app is real.
const ITEMS = [
  { group: "Sections", label: { k: "nav.loads" }, hint: { k: "nav.group.ingest" }, href: "/ingest/loads", icon: "download" },
  { group: "Actions", action: "toggle-theme", label: { k: "palette.theme" }, hint: { k: "palette.theme.hint" }, href: "#", icon: "sun" },
  { group: "Projects", label: "analytics", hint: { k: "loads.summary.workflows", n: 12 }, href: "/workflows?project=analytics", icon: "folder" },
];

test("the palette says the server's sentences in the reader's language and searches what it says", async ({ page, baseURL }) => {
  await page.context().addCookies([{ name: "brevis_lang", value: "pt-BR", url: baseURL ?? "http://localhost:8080" }]);
  await page.route("**/api/palette", (r) => r.fulfill({ json: { items: ITEMS } }));
  await page.goto("/");
  await loaded(page);
  await page.keyboard.press("Control+k");
  const list = page.locator("#palette-list");
  // What it says, in Portuguese: the section, its group, the theme action and the project's count.
  await page.fill("#palette-input", "carga");
  await expect(list).toContainText("Cargas batch")
  await expect(list).toContainText("Ingerir");
  await page.fill("#palette-input", "escuro");
  await expect(list).toContainText("Alternar tema");
  await page.fill("#palette-input", "analytics");
  await expect(list).toContainText("12");
  // Search looks in the words said now, not the ones the server would have said in English.
  await page.fill("#palette-input", "toggle");
  await expect(list).not.toContainText("Alternar tema");
});
