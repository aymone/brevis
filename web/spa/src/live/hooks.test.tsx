import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Fn = (w: "ready" | "change") => void;
const subs = new Map<string, Fn>();
const offs = vi.fn();
let up = false;
const watchers = new Set<() => void>();

vi.mock("./connection", () => ({
  subscribe: (topic: string, fn: Fn) => {
    subs.set(topic, fn);
    return () => offs(topic);
  },
  isUp: () => up,
  watchUp: (w: () => void) => {
    watchers.add(w);
    return () => void watchers.delete(w);
  },
}));

import { useFallbackInterval, useLive } from "./hooks";

let qc: QueryClient;
const wrap = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
const stale = (key: string, ageMs: number) => qc.setQueryData([key], "x", { updatedAt: Date.now() - ageMs });
const invalidated = (key: string) => qc.getQueryState([key])!.isInvalidated;

beforeEach(() => {
  qc = new QueryClient();
  subs.clear();
  offs.mockClear();
  up = false;
});
afterEach(() => watchers.clear());

describe("useLive", () => {
  it("invalidates its keys when the server says the topic changed, however fresh the data", () => {
    renderHook(() => useLive(["runs"], [["a"]]), { wrapper: wrap });
    stale("a", 100);
    act(() => subs.get("runs")!("change"));
    expect(invalidated("a")).toBe(true);
  });

  it("on `ready` refetches only data older than 3 s (the screen fetched on mount a moment ago)", () => {
    renderHook(() => useLive(["runs"], [["old"], ["new"]]), { wrapper: wrap });
    stale("old", 10_000);
    stale("new", 500);
    act(() => subs.get("runs")!("ready"));
    expect(invalidated("old")).toBe(true);
    expect(invalidated("new")).toBe(false);
  });

  it("subscribes only while mounted, and not at all for no topics", () => {
    const { unmount } = renderHook(() => useLive(["runs", "nav"], [["a"]]), { wrapper: wrap });
    expect([...subs.keys()].sort()).toEqual(["nav", "runs"]);
    unmount();
    expect(offs).toHaveBeenCalledTimes(2);
    subs.clear();
    renderHook(() => useLive([], [["a"]]), { wrapper: wrap });
    expect(subs.size).toBe(0);
  });

  it("does not resubscribe when the caller passes equal arrays on every render", () => {
    const { rerender } = renderHook(() => useLive(["runs"], [["a"]]), { wrapper: wrap });
    rerender();
    rerender();
    expect(offs).not.toHaveBeenCalled();
  });
});

describe("useFallbackInterval", () => {
  it("is false while the stream is up and the interval while it is down, following changes", () => {
    const { result } = renderHook(() => useFallbackInterval(5_000));
    expect(result.current).toBe(5_000);
    up = true;
    act(() => watchers.forEach((w) => w()));
    expect(result.current).toBe(false);
    up = false;
    act(() => watchers.forEach((w) => w()));
    expect(result.current).toBe(5_000);
  });
  it("defaults to 30 s", () => {
    expect(renderHook(() => useFallbackInterval()).result.current).toBe(30_000);
  });
});
