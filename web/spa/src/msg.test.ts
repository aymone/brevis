import { describe, expect, it } from "vitest";
import { renderCat, renderMsg, type Cat, type Msg } from "./i18n";

// A catalog and a formatter stand in for the hooks: the rule is which value goes where.
const cat: Record<string, string> = {
  "run.why.step": "Stopped at {step} (exit {code})",
  "summary.read.many": "Reads {count} sources",
  "x": "{n} in {d} at {t}: {inner}",
  "inner": "inner",
};
const say = {
  t: (k: string, v: Record<string, string | number> = {}) => (cat[k] ?? `[${k}]`).replace(/\{(\w+)\}/g, (w, k2: string) => (k2 in v ? String(v[k2]) : w)),
  n: (k: string, c: number, v: Record<string, string | number> = {}) => say.t(k, { count: c, ...v }),
  fmt: { number: (v: number) => `#${v}`, duration: (ms: number) => `${ms}ms`, stamp: (iso: string, layout: string, zone = "UTC") => `${layout}:${iso}@${zone}` },
};

describe("renderMsg", () => {
  it("fills named values", () => {
    const m: Msg = { k: "run.why.step", a: { step: "load", code: 2 } };
    expect(renderMsg(m, say)).toBe("Stopped at load (exit 2)");
  });

  it("picks the plural form for the count and fills {count}", () => {
    expect(renderMsg({ k: "summary.read.many", n: 3 }, say)).toBe("Reads 3 sources");
    expect(renderMsg({ k: "summary.read.many", n: 0 }, say)).toBe("Reads 0 sources");
  });

  it("formats the typed values and says a nested message", () => {
    const m: Msg = { k: "x", a: { n: { $: "num", v: 1200 }, d: { $: "dur", ms: 90000 }, t: { $: "at", v: "2026-10-02T09:05:00Z", layout: "short", zone: "UTC" }, inner: { k: "inner" } } };
    expect(renderMsg(m, say)).toBe("#1200 in 90000ms at short:2026-10-02T09:05:00Z@UTC: inner");
  });

  it("leaves a missing value visible, as the catalog's fill does", () => {
    expect(renderMsg({ k: "run.why.step", a: { step: "load" } }, say)).toBe("Stopped at load (exit {code})");
  });
});

describe("renderCat", () => {
  it("says messages and strings one after the other, alone or as a value", () => {
    const c: Cat = { $: "cat", v: [{ k: "inner" }, ", ", "b", " ", { k: "run.why.step", a: { step: "s", code: 1 } }] };
    expect(renderCat(c, say)).toBe("inner, b Stopped at s (exit 1)");
    expect(renderMsg({ k: "x", a: { n: 1, d: "d", t: "t", inner: c } }, say)).toBe("1 in d at t: inner, b Stopped at s (exit 1)");
  });
});

describe("a summary's punctuation is the language's", () => {
  // The server sends the pieces of a summary with its separators as messages (summary.sep.*): Japanese writes no space
  // between clauses and ends with 。, English the opposite.
  const pieces = { $: "cat", v: [{ k: "a" }, { k: "summary.sep.space" }, { k: "b" }, { k: "summary.sep.comma" }, { k: "c" }, { k: "summary.sep.stop" }] } as const;
  const make = (sep: Record<string, string>) => ({
    ...say,
    t: (k: string) => ({ a: "A", b: "B", c: "C", ...sep })[k] ?? `[${k}]`,
  });
  it("joins with the language's own separators", async () => {
    const { renderCat } = await import("./i18n");
    expect(renderCat(pieces as never, make({ "summary.sep.space": " ", "summary.sep.comma": ", ", "summary.sep.stop": "." }) as never)).toBe("A B, C.");
    expect(renderCat(pieces as never, make({ "summary.sep.space": "", "summary.sep.comma": "、", "summary.sep.stop": "。" }) as never)).toBe("AB、C。");
  });
});
