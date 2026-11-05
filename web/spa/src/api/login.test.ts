import { describe, expect, it } from "vitest";
import { safeNext } from "./login";

describe("safeNext", () => {
  it("keeps a same-origin path and defaults to the front page", () => {
    expect(safeNext("/runs?state=failed")).toBe("/runs?state=failed");
    expect(safeNext(null)).toBe("/");
    expect(safeNext("")).toBe("/");
  });

  it("refuses another origin, a backslash and control characters", () => {
    for (const bad of ["//evil.com", "/\\evil.com", "https://evil.com", "/a\nb"]) expect(safeNext(bad)).toBe("/");
  });

  it("reads a bookmark from when the console lived under /app", () => {
    expect(safeNext("/app/runs?x=1")).toBe("/runs?x=1");
    expect(safeNext("/app/")).toBe("/");
    expect(safeNext("/app")).toBe("/");
    expect(safeNext("/application")).toBe("/application");
  });
});
