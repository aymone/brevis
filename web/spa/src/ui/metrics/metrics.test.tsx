import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderApp, seededClient } from "../../test/harness";
import { health, sumRate, sumSeries } from "./hooks";
import { MinuteBars, Trend } from "./charts";
import { StreamsStrip, StreamRowRate } from "./streams";
import type { Entity, Owner, StreamsMetrics } from "./types";

const owner = (o: Partial<Owner>): Owner => ({ name: "gw", state: "ok", replicas: 1, reachable: 1, lastSuccess: "2026-11-03T10:00:00Z", ...o });
const series = (n: number, v: number | null) => Array.from({ length: n }, () => v);
const entity = (over: Partial<Entity> = {}): Entity => ({
  owner: "gw", name: "clicks", present: true,
  values: { received: series(30, 6), rejected: series(30, 0), deadLetter: series(30, 0), delivered: series(30, 1) },
  covered: series(30, 60) as number[], ratePerMin: { received: 6, rejected: 0, deadLetter: 0, delivered: 1 },
  lastHour: { received: 360, rejected: 0, deadLetter: 0, delivered: 60 }, observedSec: 1800, ...over,
});
const snap = (over: Partial<StreamsMetrics> = {}): StreamsMetrics => ({
  configured: true, since: "2026-11-03T09:00:00Z", intervalSec: 5, window: { start: 1_700_000_000, minutes: 30 },
  owners: [owner({})], total: entity({ owner: "", name: "" }), entities: [{ ...entity(), gateway: "gw", path: "/v1/clicks" }], ...over,
});

describe("what a screen may claim about the source", () => {
  it("is not configured, waiting, unreachable or ready, and never invents numbers", () => {
    expect(health(false, [])).toEqual({ kind: "not_configured" });
    expect(health(true, [])).toEqual({ kind: "waiting" });
    expect(health(true, [owner({ state: "waiting" })])).toEqual({ kind: "waiting" });
    expect(health(true, [owner({ state: "unreachable" })])).toMatchObject({ kind: "unreachable" });
    expect(health(true, [owner({ state: "ok" }), owner({ name: "b", state: "partial", dropped: 4 })])).toMatchObject({ kind: "ready", dropped: 4 });
  });
  it("a partly down source is still ready, and says which part", () => {
    const h = health(true, [owner({ state: "ok" }), owner({ name: "b", state: "unreachable" })]);
    expect(h).toMatchObject({ kind: "ready", partial: [{ name: "b" }] });
  });
});

describe("sums keep the difference between unobserved and zero", () => {
  it("a minute is null only when no measure was observed in it", () => {
    const e = entity({ values: { a: [1, null, null], b: [2, 0, null] } });
    expect(sumSeries(e, ["a", "b"])).toEqual([3, 0, null]);
  });
  it("a rate is null when none is known", () => {
    expect(sumRate(entity({ ratePerMin: { a: null, b: null } }), ["a", "b"])).toBeNull();
    expect(sumRate(entity({ ratePerMin: { a: 2, b: null } }), ["a", "b"])).toBe(2);
  });
});

describe("the charts", () => {
  it("a trend is an image with the spoken summary, and an unobserved minute is a tick, not a bar", () => {
    renderApp(<Trend values={[1, null, 3]} label="Events per minute. Now: 3." />);
    const img = screen.getByRole("img", { name: "Events per minute. Now: 3." });
    expect(img.querySelectorAll("rect")).toHaveLength(3);
    expect(img.querySelectorAll("rect")[1]?.getAttribute("height")).toBe("1");
  });
  it("bars carry the same numbers in a table for everyone who cannot see them", () => {
    renderApp(<MinuteBars start={1_700_000_000} label="chart" caption="the numbers" layers={[{ key: "r", label: "Received", color: "red", values: [4, null, 9] }]} />);
    const table = screen.getByRole("table", { name: "the numbers" });
    expect(table.textContent).toContain("9");
    expect(table.textContent).toContain("[metrics.not_observed]");
    expect(screen.getByRole("img", { name: "chart" })).toBeTruthy();
  });
});

function withAnswer(answer: unknown, status = 200) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(answer), { status, headers: { "Content-Type": "application/json" } })));
}

describe("the Streams band and rows", () => {
  it("not configured says so and offers the annotations to put on the pod", async () => {
    withAnswer(snap({ configured: false, owners: [], entities: [] }));
    renderApp(<StreamsStrip />, { qc: seededClient() });
    expect(await screen.findByText("[metrics.nc.title]")).toBeTruthy();
    expect(screen.getByText(/brevis\.sh\/metrics-kind: gateway/)).toBeTruthy();
  });
  it("waiting for the first scrape draws no number", async () => {
    withAnswer(snap({ owners: [owner({ state: "waiting" })] }));
    renderApp(<StreamsStrip />, { qc: seededClient() });
    expect(await screen.findByText("[metrics.waiting]")).toBeTruthy();
  });
  it("an unreachable source says when it last answered", async () => {
    withAnswer(snap({ owners: [owner({ state: "unreachable", error: "timeout" })] }));
    renderApp(<StreamsStrip />, { qc: seededClient() });
    expect(await screen.findByText("[metrics.unreachable]")).toBeTruthy();
    expect(screen.getByText(/metrics\.error\.timeout/)).toBeTruthy();
  });
  it("a working source shows the rate now, from the server's number", async () => {
    withAnswer(snap());
    renderApp(<StreamsStrip />, { qc: seededClient() });
    expect(await screen.findByText("6")).toBeTruthy();
  });
  it("a row finds its stream by gateway and path; another path gets a dash, not a zero", async () => {
    withAnswer(snap());
    renderApp(
      <>
        <StreamRowRate gateway="gw" path="/v1/clicks" />
        <StreamRowRate gateway="gw" path="/v1/other" />
      </>,
    );
    expect(await screen.findByText("6")).toBeTruthy();
    expect(await screen.findAllByText("—")).toHaveLength(1);
  });
  it("a failing endpoint is an alert, not an empty chart", async () => {
    withAnswer({ error: "boom" }, 500);
    renderApp(<StreamsStrip />, { qc: seededClient() });
    expect(await screen.findByRole("alert")).toBeTruthy();
  });
});
