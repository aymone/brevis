import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { boot } from "./boot";

const session = { operator: "alice", lang: "pt-BR" };
const config = { brand: { title: "T", css: ":root{--accent:red}" }, version: "1", sql: false };
const messages = { lang: "pt-BR", messages: { "nav.x": "x" } };

beforeEach(() => {
  localStorage.clear();
  document.head.querySelectorAll("style[data-brand]").forEach((e) => e.remove());
  Object.defineProperty(document, "fonts", { configurable: true, value: { load: vi.fn(() => Promise.resolve([])) } });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) => {
      const body = path === "/api/session" ? session : path === "/api/config" ? config : messages;
      return { status: 200, ok: true, redirected: false, json: async () => body } as Response;
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

describe("boot", () => {
  it("puts session, config and the catalog in the cache BEFORE any screen renders, so static words never suspend", async () => {
    const qc = new QueryClient();
    await boot(qc);
    expect(qc.getQueryData(["session"])).toEqual(session);
    expect(qc.getQueryData(["config"])).toEqual(config);
    expect(qc.getQueryData(["messages", "pt-BR"])).toEqual(messages); // keyed by the session's language
  });

  it("applies the brand CSS and the language to the document before the first paint", async () => {
    await boot(new QueryClient());
    expect(document.head.querySelector("style[data-brand]")?.textContent).toBe(config.brand.css);
    expect(document.documentElement.lang).toBe("pt-BR");
  });

  it("binds the operator, so another person's remembered shapes are dropped", async () => {
    localStorage.setItem("brevis.seen.k", "{}");
    await boot(new QueryClient());
    expect(localStorage.getItem("brevis.seen.k")).toBeNull();
    expect(localStorage.getItem("brevis.seen.operator")).toMatch(/^\d+$/);
  });

  it("a font that never loads costs at most the 400 ms cap, not a blank page", async () => {
    vi.useFakeTimers();
    (document.fonts.load as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));
    const done = vi.fn();
    const p = boot(new QueryClient()).then(done);
    await vi.advanceTimersByTimeAsync(399);
    expect(done).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2);
    await p;
    expect(done).toHaveBeenCalled();
    vi.useRealTimers();
  });

  it("fails (and renders nothing) when the session cannot be read", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => ({ status: 500, ok: false, redirected: false, json: async () => ({}) }) as Response));
    await expect(boot(new QueryClient())).rejects.toMatchObject({ status: 500 });
  });
});
