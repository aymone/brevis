import { describe, expect, it } from "vitest";
import { formatDuration, formatSpan, formatUntil } from "./duration";

// ENGLISH IS THE COMPACT FORM THE CONSOLE ALWAYS HAD; the other languages say the unit in their own words (Intl).
describe("formatDuration", () => {
  it("keeps English exactly as it was", () => {
    expect(formatDuration("en", 400)).toBe("400ms");
    expect(formatDuration("en", 1500)).toBe("1.5s");
    expect(formatDuration("en", 125_000)).toBe("2m 05s");
    expect(formatDuration("en", 3_840_000)).toBe("1h 04m");
  });

  it("rounds a tie to the even digit, as Go did (1250ms is 1.2s)", () => {
    expect(formatDuration("en", 1250)).toBe("1.2s");
    expect(formatDuration("en", 1350)).toBe("1.4s");
  });

  it("says the units in the reader's language", () => {
    expect(formatDuration("ru", 1500)).toBe("1,5 с");
    expect(formatDuration("ru", 125_000)).toBe("2 мин 05 с");
    expect(formatDuration("ru", 3_840_000)).toBe("1 ч 04 мин");
    expect(formatDuration("ko", 125_000)).toBe("2분 05초");
    expect(formatDuration("zh-CN", 3_840_000)).toBe("1小时 04分钟");
    expect(formatDuration("de", 1500)).toBe("1,5 Sek.");
  });
});

describe("formatSpan", () => {
  it("is minutes under an hour, hours under a day, then days (English as it was)", () => {
    expect(formatSpan("en", 180_000)).toBe("3 min");
    expect(formatSpan("en", 5 * 3_600_000)).toBe("5h");
    expect(formatSpan("en", 2 * 86_400_000)).toBe("2d");
  });
  it("says them in the reader's language", () => {
    expect(formatSpan("ru", 180_000)).toBe("3 мин");
    expect(formatSpan("ko", 5 * 3_600_000)).toBe("5시간");
  });
});

describe("formatUntil", () => {
  it("rounds a wait to what a person plans by", () => {
    expect(formatUntil("en", 30_000)).toBe("<1m");
    expect(formatUntil("en", 9 * 3_600_000 + 28 * 60_000)).toBe("9h 28m");
    expect(formatUntil("en", 5 * 3_600_000)).toBe("5h");
    expect(formatUntil("en", 3 * 86_400_000)).toBe("3d");
  });
  it("says the unit in the reader's language", () => {
    expect(formatUntil("ru", 9 * 3_600_000 + 28 * 60_000)).toBe("9 ч 28 мин");
  });
});
