import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowRow, WorkflowsList } from "../api/workflows";
import { renderApp } from "../test/harness";
import WorkflowsRoute from "./workflows";

// The event stream is not under test here: a screen with no live connection polls.
vi.mock("../live/connection", () => ({ subscribe: () => () => {}, isUp: () => true, watchUp: () => () => {} }));

const row = (slug: string): WorkflowRow => ({
  slug, project: "p", steps: 2, failing: false, inFlight: false, hasParams: false, runsIn: [], connects: [], hasSchedule: false, active: true,
  cron: "", schedule: "", timezone: "UTC", catchup: false, nextRun: null, lastRunAt: null, totalRuns: 1, recent: [], tags: [],
});
const list = (rows: WorkflowRow[], q = ""): WorkflowsList => ({
  filter: { q, state: "", active: "", lang: "", tags: [], project: "", sort: "", dir: "asc", page: 1, perPage: 25 },
  rows, counts: {}, tags: [], langs: [], total: 3, filtered: rows.length,
});

let calls: string[];
let answer: (url: string) => Promise<WorkflowsList>;

beforeEach(() => {
  calls = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    calls.push(url);
    const body = await answer(url);
    return { status: 200, ok: true, redirected: false, json: async () => body } as Response;
  }));
});
afterEach(() => vi.unstubAllGlobals());

const slugs = () => [...document.querySelectorAll("tbody a[href^='/workflows/']")].map((a) => a.textContent);
const search = (v: string) => fireEvent.change(screen.getByRole("searchbox"), { target: { value: v } });

describe("Workflows screen", () => {
  it("a search that finds nothing shows the empty state instead of crashing (React error #300: a hook after the early return)", async () => {
    answer = async (u) => (u.includes("q=zzz") ? list([], "zzz") : list([row("a"), row("b"), row("c")]));
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const { container } = renderApp(<WorkflowsRoute />, { at: "/workflows" });
    await waitFor(() => expect(slugs()).toEqual(["a", "b", "c"]));

    search("zzz"); // rows go 3 -> 0 on the same mounted table
    await waitFor(() => expect(calls.some((c) => c.includes("q=zzz"))).toBe(true));
    await waitFor(() => expect(slugs()).toEqual([]));

    expect(container.querySelector("table")).toBeNull();
    expect(container.textContent).not.toBe(""); // the page is still there, not blank
    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(errors.mock.calls.flat().join(" ")).not.toMatch(/rendered fewer hooks|Minified React error #300/);

    search(""); // and back: 0 -> 3
    await waitFor(() => expect(slugs()).toEqual(["a", "b", "c"]));
  });

  it("a refetch of the same key keeps the rows on screen: no skeleton, not busy", async () => {
    answer = async () => list([row("a"), row("b")]);
    const { container, qc } = renderApp(<WorkflowsRoute />, { at: "/workflows" });
    await waitFor(() => expect(slugs()).toEqual(["a", "b"]));
    const before = calls.length;

    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    answer = async () => (await gate, list([row("a"), row("b"), row("c")]));
    act(() => void qc.invalidateQueries({ queryKey: ["workflows"] })); // sync: an async act left open poisons the next test
    await waitFor(() => expect(calls.length).toBe(before + 1)); // the refetch is in flight

    expect(slugs()).toEqual(["a", "b"]);
    expect(container.querySelector(".sk-ghost")).toBeNull();
    expect(container.querySelector("[aria-busy='true']")).toBeNull();

    release();
    await waitFor(() => expect(slugs()).toEqual(["a", "b", "c"]));
  });

  it("loads once on mount (no duplicate request)", async () => {
    answer = async () => list([row("a")]);
    renderApp(<WorkflowsRoute />, { at: "/workflows" });
    await waitFor(() => expect(slugs()).toEqual(["a"]));
    expect(calls.filter((c) => c.startsWith("/api/workflows"))).toHaveLength(1);
  });

  it("the first load draws placeholder rows, not an empty state", async () => {
    answer = () => new Promise(() => {});
    const { container } = renderApp(<WorkflowsRoute />, { at: "/workflows" });
    await waitFor(() => expect(container.querySelector(".sk-ghost")).not.toBeNull());
    expect(container.querySelector("[aria-busy='true']")).not.toBeNull();
  });
});
