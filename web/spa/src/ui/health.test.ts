import { expect, test } from "vitest";
import { HEALTH_ORDER, healthKind, severity } from "./health";

test("a stream's health kind is what the server said, and unknown when it said nothing", () => {
  expect(healthKind({ state: "unknown", reason: "not_reported" })).toBe("unknown");
  expect(healthKind({ state: "", reason: "" })).toBe("unknown");
  expect(healthKind(undefined)).toBe("unknown");
});

test("failing streams come first, then degraded, then the rest; a stream with no health is one of the rest", () => {
  const rows = [
    { n: "a", h: { state: "healthy", reason: "delivering" } },
    { n: "b", h: { state: "degraded", reason: "retrying" } },
    { n: "c", h: undefined },
    { n: "d", h: { state: "failing", reason: "saturating" } },
    { n: "e", h: { state: "quiet", reason: "no_traffic" } },
    { n: "f", h: { state: "failing", reason: "rejected_most" } },
  ];
  const order = [...rows].sort((x, y) => severity(x.h) - severity(y.h)).map((r) => r.n);
  expect(order).toEqual(["d", "f", "b", "a", "c", "e"]);
});

test("the summary lists states from what needs a person to what is fine", () => {
  expect(HEALTH_ORDER.slice(0, 3)).toEqual(["failing", "degraded", "healthy"]);
  expect(healthKind({ state: "failing", reason: "saturating" })).toBe("failing");
});
