import { describe, expect, it } from "vitest";
import { category } from "./plural";

// The same table is in web/i18n/plural_test.go: the app and the server must pick the same form.
describe("category", () => {
  const table: [string, number, string][] = [
    ["en", 0, "other"], ["en", 1, "one"], ["en", 2, "other"], ["pt-BR", 21, "other"], ["ja", 1, "one"],
    ["ru", 0, "many"], ["ru", 1, "one"], ["ru", 2, "few"], ["ru", 4, "few"], ["ru", 5, "many"],
    ["ru", 11, "many"], ["ru", 12, "many"], ["ru", 14, "many"], ["ru", 21, "one"], ["ru", 22, "few"],
    ["ru", 25, "many"], ["ru", 101, "one"], ["ru", 111, "many"], ["ru", 112, "many"], ["ru", 1002, "few"],
  ];
  it.each(table)("%s %d is %s", (lang, n, want) => expect(category(lang, n)).toBe(want));
});
