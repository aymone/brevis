import { describe, expect, it } from "vitest";
import { fixed } from "./fixed";

describe("fixed", () => {
  it("rounds as Go's strconv does: the exact double, a tie to the even digit", () => {
    expect(fixed("en", 6.85, 1)).toBe("6.8"); // the double below 6.85
    expect(fixed("en", 1.25, 1)).toBe("1.2"); // an exact tie goes to even
    expect(fixed("en", 1.35, 1)).toBe("1.4"); // the double above 1.35
    expect(fixed("en", 0.5, 0)).toBe("0");
    expect(fixed("en", 1.5, 0)).toBe("2");
    expect(fixed("en", 12, 1)).toBe("12.0");
    expect(fixed("en", 5.65, 1)).toBe("5.7"); // 56.5 after the multiplication, but not a tie
    expect(fixed("en", 2.5, 0)).toBe("2");
    expect(fixed("en", 3.5, 0)).toBe("4");
    expect(fixed("en", 0.4, 1)).toBe("0.4");
  });
  it("writes the reader's decimal mark", () => {
    expect(fixed("pt-BR", 2.5, 1)).toBe("2,5");
    expect(fixed("en", 2.5, 1)).toBe("2.5");
  });
});
