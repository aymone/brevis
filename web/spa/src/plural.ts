/**
 * The plural category of n in lang: "one", "few", "many" or "other".
 *
 * Mirrors web/i18n/plural.go (plural.test.ts holds the same table). Most languages
 * here need only one (n === 1) and other; Russian needs more ("1 запуск",
 * "2 запуска", "5 запусков", "21 запуск"). Add a language here AND there.
 */
export function category(lang: string, n: number): "one" | "few" | "many" | "other" {
  n = Math.abs(Math.trunc(n));
  if (lang === "ru") {
    const m10 = n % 10;
    const m100 = n % 100;
    if (m10 === 1 && m100 !== 11) return "one";
    if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return "few";
    return "many";
  }
  return n === 1 ? "one" : "other";
}
