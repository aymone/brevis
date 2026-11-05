import { expect, routes, loaded, test } from "./support";

// EVERY SCREEN, IN EVERY LANGUAGE. audit.spec.ts walks the screens in English; this one walks
// them in all eleven, because that is where a missing key (`[some.key]` on the page), a
// placeholder the translation dropped (`{count}` printed as written) and a long German or
// Russian word that pushes the page sideways show up. Two widths: the phone, where the bars
// are tightest, and the desktop.
const LANGS = ["en", "es", "pt-BR", "fr", "de", "zh-CN", "ja", "ru", "ko", "hi", "id"];
const WIDTHS = [
  { w: 390, h: 844 },
  { w: 1440, h: 900 },
];

for (const lang of LANGS) {
  test(`every screen reads cleanly in ${lang}`, async ({ browser, seed, baseURL }) => {
    test.setTimeout(180_000);
    const problems: string[] = [];
    for (const { w, h } of WIDTHS) {
      // The language is the cookie the sidebar's picker sets: the server negotiates from it.
      const ctx = await browser.newContext({ viewport: { width: w, height: h } });
      await ctx.addCookies([{ name: "brevis_lang", value: lang, url: baseURL ?? "http://localhost:8080" }]);
      const page = await ctx.newPage();
      for (const route of routes(seed)) {
        const tag = `${route} @${w}`;
        await page.goto(route);
        await loaded(page).catch((e: Error) => problems.push(`${tag}: not loaded (${e.message.split("\n")[0]})`));
        const res = await page.evaluate(async () => {
          await document.fonts.ready;
          const text = document.body.innerText;
          return {
            lang: document.documentElement.lang,
            over: document.documentElement.scrollWidth - document.documentElement.clientWidth,
            keys: text.match(/\[[a-z]+(\.[a-z_0-9]+)+\]/g)?.slice(0, 3) ?? [],
            holes: text.match(/\{[a-z_]+\}/g)?.slice(0, 3) ?? [],
          };
        });
        if (res.lang && res.lang !== lang && lang !== "en") problems.push(`${tag}: page language is ${res.lang}`);
        if (res.over > 0) problems.push(`${tag}: sideways overflow ${res.over}px`);
        if (res.keys.length) problems.push(`${tag}: missing keys ${res.keys}`);
        if (res.holes.length) problems.push(`${tag}: unfilled placeholders ${res.holes}`);
      }
      await ctx.close();
    }
    expect(problems).toEqual([]);
  });
}
