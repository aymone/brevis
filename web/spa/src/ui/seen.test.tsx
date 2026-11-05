import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as rows from "./remembered";
import { bindOperator, clearSeen, recall, remember, useSeen } from "./seen";

const stored = () => Object.fromEntries(Object.keys(localStorage).map((k) => [k, localStorage.getItem(k)]));
// A private window: every touch of the storage throws.
const refuse = () =>
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("denied");
  }) &&
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("denied");
  });

beforeEach(() => localStorage.clear());

describe("remembered rows", () => {
  it("stores only the number, under the brevis.seen prefix", () => {
    rows.remember("workflows.rows", 7);
    expect(stored()).toEqual({ "brevis.seen.rows.workflows.rows": "7" });
  });
  it("recalls it, capped, and falls back for anything that is not a positive integer", () => {
    rows.remember("a", 500);
    expect(rows.recalled("a", 10)).toBe(100);
    for (const bad of ["abc", "-3", "0", "2.5", ""]) {
      localStorage.setItem("brevis.seen.rows.b", bad);
      expect(rows.recalled("b", 10)).toBe(10);
    }
    expect(rows.recalled("none", 10)).toBe(10);
  });
  it("an empty result is not remembered (the next skeleton would draw nothing)", () => {
    rows.remember("a", 0);
    expect(stored()).toEqual({});
  });
  it("survives a storage that throws", () => {
    refuse();
    expect(() => rows.remember("a", 3)).not.toThrow();
    expect(rows.recalled("a", 10)).toBe(10);
  });
});

describe("seen shapes", () => {
  it("recall merges what was kept over the fallback, and returns the fallback on garbage", () => {
    remember("k", { rows: 4 });
    expect(recall("k", { rows: 1, pages: 1 })).toEqual({ rows: 4, pages: 1 });
    localStorage.setItem("brevis.seen.k", "{oops");
    expect(recall("k", { rows: 1 })).toEqual({ rows: 1 });
  });

  it("clearSeen removes everything under the prefix and nothing else", () => {
    remember("a", { n: 1 });
    rows.remember("b", 2);
    localStorage.setItem("brevis.theme", "dark");
    clearSeen();
    expect(stored()).toEqual({ "brevis.theme": "dark" });
  });

  it("bindOperator stores a number derived from the name, never the name", () => {
    bindOperator("alice@example.com");
    const v = localStorage.getItem("brevis.seen.operator")!;
    expect(v).toMatch(/^\d+$/);
    expect(JSON.stringify(stored())).not.toContain("alice");
  });

  it("the same operator keeps what was remembered; another one starts from nothing", () => {
    bindOperator("alice");
    remember("k", { rows: 9 });
    bindOperator("alice");
    expect(recall("k", { rows: 1 })).toEqual({ rows: 9 });
    bindOperator("bob");
    expect(recall("k", { rows: 1 })).toEqual({ rows: 1 });
  });

  it("every function tolerates a storage that throws", () => {
    refuse();
    expect(() => {
      remember("k", { a: 1 });
      clearSeen();
      bindOperator("x");
    }).not.toThrow();
    expect(recall("k", { a: 0 })).toEqual({ a: 0 });
  });
});

describe("useSeen", () => {
  it("draws the remembered shape before the answer, the live one after, and remembers the live one", () => {
    remember("s", { n: 5 });
    const { result, rerender } = renderHook(({ live }: { live?: { n: number } }) => useSeen("s", live, { n: 1 }), { initialProps: {} as { live?: { n: number } } });
    expect(result.current).toEqual({ n: 5 });
    rerender({ live: { n: 2 } });
    expect(result.current).toEqual({ n: 2 });
    expect(recall("s", { n: 0 })).toEqual({ n: 2 });
  });
});
