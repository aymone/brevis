import { describe, expect, it } from "vitest";
import { trendDay, trendValue } from "./trend";

describe("the load trend's numbers", () => {
  it("writes a count with its magnitude", () => {
    expect(trendValue("en", "count", 2000)).toBe("2.0k");
    expect(trendValue("en", "count", 1_500_000)).toBe("1.5M");
    expect(trendValue("en", "count", 999)).toBe("999");
    expect(trendValue("pt-BR", "count", 2500)).toBe("2,5k");
  });
  it("writes a size and seconds", () => {
    expect(trendValue("en", "size", 200)).toBe("200 B");
    expect(trendValue("en", "size", 3 * 2 ** 20)).toBe("3.0 MB");
    expect(trendValue("en", "seconds", 0.4)).toBe("0.4s");
    expect(trendValue("en", "seconds", 12)).toBe("12s");
    expect(trendValue("en", "seconds", 125)).toBe("2m 5s");
    expect(trendValue("en", "seconds", 0)).toBe("0s");
  });
  it("keeps the day first in English, with the year when asked", () => {
    expect(trendDay("en", "2026-03-09")).toBe("9 Mar");
    expect(trendDay("en", "2026-09-28", true)).toBe("28 Sep 2026");
    expect(trendDay("pt-BR", "2026-10-02")).toMatch(/^2 de out/);
  });

  it("writes seconds with the reader's unit (English as it was)", () => {
    expect(trendValue("en", "seconds", 12)).toBe("12s");
    expect(trendValue("en", "seconds", 0.4)).toBe("0.4s");
    expect(trendValue("en", "seconds", 125)).toBe("2m 5s");
    expect(trendValue("ru", "seconds", 12)).toBe("12 с");
    expect(trendValue("ru", "seconds", 125)).toBe("2 мин 5 с");
  });
});
