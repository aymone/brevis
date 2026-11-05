import type { ReactNode } from "react";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { useFormat, useT } from "./i18n";

function setup(messages: Record<string, unknown>, lang = "en") {
  const qc = new QueryClient();
  qc.setQueryData(["session"], { operator: "o", lang });
  qc.setQueryData(["messages", lang], { lang, messages });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { wrapper };
}

describe("useT", () => {
  const { wrapper } = setup({
    "nav.projects": "Projects",
    "run.since": "since {at} by {who}",
    "nav.attention": { one: "{count} needs attention", other: "{count} need attention" },
  });
  const t = () => renderHook(() => useT(), { wrapper }).result.current;

  it("returns the message, fills named placeholders and leaves an unknown one as written", () => {
    expect(t().t("nav.projects")).toBe("Projects");
    expect(t().t("run.since", { at: "9am", who: "me" })).toBe("since 9am by me");
    expect(t().t("run.since", { at: "9am" })).toBe("since 9am by {who}");
    expect(t().t("run.since", { at: 0, who: "x" })).toBe("since 0 by x"); // 0 is a value, not a missing one
  });

  it("a missing key shows [key], the server's own mark, and never throws", () => {
    expect(t().t("nope.key")).toBe("[nope.key]");
    expect(t().n("nope.key", 2)).toBe("[nope.key]");
  });

  it("n() picks the one/other form and fills {count}", () => {
    expect(t().n("nav.attention", 1)).toBe("1 needs attention");
    expect(t().n("nav.attention", 0)).toBe("0 need attention");
    expect(t().n("nav.attention", 3)).toBe("3 need attention");
  });

  it("a string message ignores the count", () => {
    expect(t().n("nav.projects", 5)).toBe("Projects");
  });
});

describe("useFormat", () => {
  const { wrapper } = setup({}, "en");
  const f = () => renderHook(() => useFormat(), { wrapper }).result.current;
  it("writes durations as the server's Elapsed does", () => {
    const { duration } = f();
    expect([duration(250), duration(1500), duration(125_000), duration(3_840_000)]).toEqual(["250ms", "1.5s", "2m 05s", "1h 04m"]);
  });
  it("says a relative time in words", () => {
    const now = Date.parse("2026-01-01T12:00:00Z");
    expect(f().since("2026-01-01T11:58:00Z", now)).toBe("2 minutes ago");
  });
  it("shows the UTC instant with its zone", () => {
    expect(f().utc("2026-01-01T12:00:00Z")).toMatch(/UTC$/);
  });
});
