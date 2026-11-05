import type { Page } from "@playwright/test";
import { expect, loaded, mockStreamsList, routes, test } from "./support";

// ENGLISH THAT NO CATALOG OWNS. languages.spec.ts proves every key exists and fits; it cannot see a sentence that
// never went through a key (a label written in the component, a library's own screen-reader text, a word the
// server stored). Those read fine in English and stay English in the other ten languages, and nothing fails. So
// this spec opens the screens in languages written in another script, where English cannot hide behind a shared
// spelling, and fails on any visible text or attribute that reads as English prose.
//
// "Reads as English prose" is a heuristic on purpose: the node has two or more of the commonest English function
// words, as whole words. Identifiers, paths, commands and values (anything with code punctuation), one-word terms
// (Workflows, Stream, Dead letter: the glossary) and units never match, so what is left is a sentence or a label
// nobody translated.
const STOP = new Set("the and of to for with is are not no this that from in on or be it as at by can your you will".split(" "));
const LANGS = ["ru", "ja"];

// What may be English on a translated screen, and why. Each entry is DATA the engine or the operator wrote, never
// a sentence the app says.
const ALLOWED: { why: string; re: RegExp }[] = [
  { why: "the engine's own run log and error text, stored in the database in English at run time", re: /\b(exited with code|was (skipped|failed|canceled)|no step it depends on|command not found|not re-run)\b/i },
  { why: "a parameter's description, written by the workflow's author (examples/list-params.yaml)", re: /\bthe windows to backfill\b/i },
];

/** The visible English-looking text of the page: [where, text]. */
async function prose(page: Page): Promise<string[]> {
  const found = await page.evaluate((stop: string[]) => {
    const STOP = new Set(stop);
    const out: string[] = [];
    const reads = (s: string) => {
      // code punctuation: a path, a command, an identifier, a value; not a sentence
      if (/[_/\\=${}|;<>`]/.test(s)) return false;
      const words = s.match(/[A-Za-z']+/g) ?? [];
      return words.filter((w) => STOP.has(w.toLowerCase())).length >= 2;
    };
    const visible = (el: Element) => {
      try {
        return el.checkVisibility({ checkVisibilityCSS: true });
      } catch {
        return true;
      }
    };
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let n: Node | null; (n = walker.nextNode()); ) {
      const el = n.parentElement;
      const s = (n.textContent ?? "").replace(/\s+/g, " ").trim();
      if (!el || !s || ["SCRIPT", "STYLE", "NOSCRIPT"].includes(el.tagName) || !visible(el)) continue;
      if (reads(s)) out.push(`text: ${s}`);
    }
    for (const el of document.querySelectorAll("[title],[aria-label],[placeholder],[alt],[aria-description]")) {
      for (const a of ["title", "aria-label", "placeholder", "alt", "aria-description"]) {
        const v = (el.getAttribute(a) ?? "").trim();
        if (v && reads(v)) out.push(`${a}: ${v}`);
      }
    }
    return out;
  }, [...STOP]);
  return [...new Set(found)].filter((f) => !ALLOWED.some((a) => a.re.test(f)));
}

for (const lang of LANGS) {
  test.describe(`no untranslated prose in ${lang}`, () => {
    test.beforeEach(async ({ context, baseURL }) => {
      await context.addCookies([{ name: "brevis_lang", value: lang, url: baseURL ?? "http://localhost:8080" }]);
    });

    test("the screens", async ({ page, seed }) => {
      test.setTimeout(120_000);
      // The two lists the demo stack may leave empty are drawn with something in them, so their rows are read too.
      await mockStreamsList(page);
      const problems: string[] = [];
      for (const route of [...routes(seed), "/sql", "/workflows/e2e_nightly", "/nope-not-a-screen"]) {
        await page.goto(route);
        // The SQL console and the not-found page have no title to wait for: the shell is what says they arrived.
        if (route === "/sql" || route.includes("nope")) await expect(page.locator("main")).toBeVisible();
        else await loaded(page);
        for (const p of await prose(page)) problems.push(`${route} -> ${p}`);
      }
      expect(problems).toEqual([]);
    });

    test("the states a screen opens into", async ({ page, seed }) => {
      test.setTimeout(120_000);
      const problems: string[] = [];
      const read = async (where: string) => {
        for (const p of await prose(page)) problems.push(`${where} -> ${p}`);
      };
      // The graph's step inspector and React Flow's own controls (their default screen-reader text is English).
      await page.goto(`/workflows/${seed.slug}`);
      await loaded(page);
      await page.locator(".dag-name").first().click({ force: true });
      await expect(page.locator(".dag-insp")).toBeVisible();
      await read("step inspector");
      // Its row labels are single words, which the prose heuristic cannot see: they must not be the English ones.
      const labels = await page.locator(".dag-insp dt").allInnerTexts();
      expect(labels.filter((l) => ["step", "state", "command", "duration", "attempt", "exit code", "runtime", "tools"].includes(l))).toEqual([]);
      // The run-now drawer, open on a workflow that has a schedule (so it asks which slot).
      await page.goto("/workflows/e2e_nightly");
      await loaded(page);
      await page.locator('main button[aria-haspopup="dialog"]').first().click();
      await expect(page.locator("dialog[open]")).toBeVisible();
      await read("run-now drawer");
      await page.keyboard.press("Escape");
      if (seed.withParams) {
        // The drawer of a workflow with parameters: their names and descriptions are the author's, the rest is ours.
        await page.goto(`/workflows/${seed.withParams}`);
        await loaded(page);
        await page.locator('main button[aria-haspopup="dialog"]').first().click();
        await expect(page.locator("dialog[open]")).toBeVisible();
        await read("run-now drawer with parameters");
        await page.keyboard.press("Escape");
      }
      await page.keyboard.press("Control+k");
      await expect(page.locator("dialog[open], [role=dialog]").first()).toBeVisible();
      await read("palette");
      await page.keyboard.press("Escape");
      await expect(page.locator("dialog[open], [role=dialog]")).toHaveCount(0);
      // The shortcut is a key listener the app (re)attaches as the palette closes: on a loaded machine the first press
      // can land before it is back, so the press is retried until the dialog is there (and never pressed while it is).
      const dialog = page.locator("dialog[open], [role=dialog]").first();
      await expect(async () => {
        if (!(await dialog.isVisible())) await page.keyboard.press("Shift+?");
        await expect(dialog).toBeVisible({ timeout: 1500 });
      }).toPass({ timeout: 10_000 });
      await read("shortcuts");
      expect(problems).toEqual([]);
    });
  });
}
