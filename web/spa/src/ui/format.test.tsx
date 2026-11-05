import type { ReactNode } from "react";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { useFormat } from "../i18n";
import { useAutoValue, usePeriod, usePhaseDetail } from "./format";

const messages = {
  "run.slot": "Slot",
  "run.phase.waiting": "waiting {span}",
  "run.phase.succeeded": "succeeded",
  "state.failed": "failed",
  "state.canceled": "canceled",
  "state.on_time": "on time",
  "runs.period.from": "from {at}",
  "runs.period.until": "until {at}",
};

function hook<T>(use: () => T, lang = "en"): T {
  const qc = new QueryClient();
  qc.setQueryData(["session"], { operator: "o", lang });
  qc.setQueryData(["messages", lang], { lang, messages });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return renderHook(use, { wrapper }).result.current;
}

describe("a run's phase detail", () => {
  const detail = hook(usePhaseDetail);
  it("writes the instant, the slot and the durations", () => {
    expect(detail({ kind: "at", at: "2026-10-15T04:00:00Z" })).toBe("2026-10-15 04:00:00 UTC");
    expect(detail({ kind: "slot", at: "2026-10-15T04:00:00Z" })).toBe("slot 2026-10-15 04:00:00 UTC");
    expect(detail({ kind: "late", ms: 2000 })).toBe("+2.0s");
    expect(detail({ kind: "waiting", ms: 600_000 })).toBe("waiting 10m 00s");
    expect(detail({ kind: "ran", ms: 90_000 })).toBe("1m 30s");
    expect(detail(null)).toBe("");
  });
  it("says how it ended, with the instant when there is one", () => {
    expect(detail({ kind: "ended", state: "success" })).toBe("succeeded");
    expect(detail({ kind: "ended", state: "failed", at: "2026-10-15T04:00:31Z" })).toBe("failed · 2026-10-15 04:00:31 UTC");
    expect(detail({ kind: "ended", state: "canceled" })).toBe("canceled");
  });
});

describe("an auto param's value", () => {
  const value = hook(useAutoValue);
  it("is an instant, a lateness or the value itself", () => {
    const a = { key: "k", note: { k: "x" }, value: "", at: null, delaySeconds: null };
    expect(value({ ...a, at: "2026-03-11T01:00:00Z" })).toBe("2026-03-11 01:00:00 UTC");
    expect(value({ ...a, delaySeconds: 0 })).toBe("on time");
    expect(value({ ...a, delaySeconds: 2220 })).toBe("37m 00s");
    expect(value({ ...a, value: "false" })).toBe("false");
  });
});

describe("the runs filter's period", () => {
  it("says one end with its word and both with an arrow", () => {
    const from = "2026-11-05T10:00:00Z";
    expect(hook(() => usePeriod(from, undefined))).toMatch(/^from Nov 0?5 /);
    expect(hook(() => usePeriod(undefined, from))).toMatch(/^until /);
    expect(hook(() => usePeriod(from, "2026-11-05T11:00:00Z"))).toContain(" → ");
    expect(hook(() => usePeriod())).toBe("");
  });
});

describe("a stamp", () => {
  it("is written in English without the comma Intl adds before the time", () => {
    const { stamp } = hook(useFormat);
    expect(stamp("2026-10-02T09:05:00Z", "short", "UTC")).toBe("Oct 02 09:05");
    expect(stamp("2026-10-02T09:05:00Z", "brief", "UTC")).toBe("Oct 2 09:05");
  });
});
